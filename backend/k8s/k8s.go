// Package k8s runs workloads as Kubernetes pods — one runner-supervised pod
// per workload, the third isolation backend behind the seam (constitution
// III). The workload sees the same SOULREALM_* env contract as under native:
// the credential arrives as a Secret mounted read-only (never touching host
// disk), scratch is an in-pod emptyDir workdir, and loopback NATS URLs are
// rewritten to the node's host alias (backend/natsurl). The artifact ships
// as a per-run OCI image — one layer on a CA-trusted base, pushed
// digest-pinned to the operator's registry — so the kubelet pulls it like
// any image (specs/004 research D2). Supervision is a watch established
// BEFORE the pod exists, capturing the container termination state on every
// update: after the pod object is deleted that state is unobservable
// (research D4, measured). Like every backend it publishes no ops and owns
// no control channel; lifecycle belongs to the runner (constitutions I and V).
package k8s

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/jwt/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	"github.com/impire-io/soulrealm/backend"
	"github.com/impire-io/soulrealm/backend/native"
	"github.com/impire-io/soulrealm/backend/natsurl"
)

// Defaults for the node-side backend configuration. None of these may appear
// in a declaration — they are how THIS node isolates, never what a workload is.
const (
	DefaultNamespace = "default"
	DefaultBaseImage = "alpine:3.22" // carries a CA trust store (research D6)
)

// In-pod layout: scratch is an emptyDir at the workload's working directory
// (native parity: cwd = scratch); the credential is a Secret mounted
// read-only; the artifact is the per-run image's entrypoint.
const (
	podPrefix      = "soulrealm-"
	guestScratch   = "/scratch"
	guestCredsDir  = "/creds"
	guestCredsPath = guestCredsDir + "/nats.creds"
	guestWorkload  = "/workload"

	managedByKey   = "app.kubernetes.io/managed-by"
	managedByValue = "soulrealm"
)

// stopGrace mirrors the native backend's SIGTERM→SIGKILL grace, expressed as
// the pod's termination grace. A var so tests can shorten it.
var stopGrace = 5 * time.Second

// ImagePublisher packages resolved artifact bytes for the cluster and
// returns a digest-pinned image reference. The production implementation is
// the OCI publisher (image.go); unit tests inject a stub.
type ImagePublisher interface {
	Publish(ctx context.Context, artifact []byte, tag string) (ref string, err error)
}

// Backend launches workloads as pods on one configured cluster + namespace.
// All fields are node-side configuration (constitution III).
type Backend struct {
	// Client speaks to the cluster; kubernetes.Interface so the hermetic
	// tests inject the fake clientset (research D1).
	Client kubernetes.Interface
	// Namespace all workload pods and Secrets live in.
	Namespace string
	// Registry is the OCI repository prefix per-run artifact images are
	// pushed to and pulled from (e.g. "localhost:5001/soulrealm"). Required.
	Registry string
	// BaseImage is the CA-trusted base the artifact is layered onto.
	BaseImage string
	// HostAlias is the address at which pods reach this node; loopback NATS
	// URLs are rewritten to it. Empty means the realm's NATS is routable.
	HostAlias string
	// Images overrides the publisher (tests); nil uses the OCI publisher.
	Images ImagePublisher
}

// New returns a k8s backend for the given client with default node-side
// configuration. Registry must be set by the caller.
func New(client kubernetes.Interface) *Backend { return &Backend{Client: client} }

func (b *Backend) namespace() string {
	if b.Namespace != "" {
		return b.Namespace
	}
	return DefaultNamespace
}

func (b *Backend) baseImage() string {
	if b.BaseImage != "" {
		return b.BaseImage
	}
	return DefaultBaseImage
}

func (b *Backend) images() ImagePublisher {
	if b.Images != nil {
		return b.Images
	}
	return &registryPublisher{registry: b.Registry, base: b.baseImage()}
}

// elfMagic is the pre-launch platform guard: a non-ELF artifact fails
// unreadably inside the pod (research spike B, measured), so it is refused
// node-side before any registry or cluster call.
var elfMagic = []byte{0x7f, 'E', 'L', 'F'}

// Start packages the artifact, creates the credential Secret and the pod,
// and begins supervision. It does not block. Any failure unwinds what was
// created — nothing is left on the cluster.
func (b *Backend) Start(ctx context.Context, spec backend.LaunchSpec) (backend.Handle, error) {
	if b.Client == nil {
		return nil, fmt.Errorf("k8s: no cluster client configured")
	}
	if b.Registry == "" {
		return nil, fmt.Errorf("k8s: no registry configured (SOULREALM_K8S_REGISTRY)")
	}
	name := podName(spec.ScratchDir)

	artifact, err := os.ReadFile(spec.Artifact)
	if err != nil {
		return nil, fmt.Errorf("k8s: read artifact: %w", err)
	}
	if !bytes.HasPrefix(artifact, elfMagic) {
		return nil, fmt.Errorf("k8s: artifact %s is not an ELF binary (platform mismatch — build GOOS=linux for the cluster)", spec.Artifact)
	}
	if b.HostAlias == "" && natsurl.HasLoopback(spec.Cred.NatsServers) {
		return nil, fmt.Errorf("k8s: NATS servers include loopback but no host alias is configured — pods cannot reach the node's loopback (SOULREALM_K8S_HOST_ALIAS)")
	}

	ref, err := b.images().Publish(ctx, artifact, name)
	if err != nil {
		return nil, fmt.Errorf("k8s: publish artifact image: %w", err)
	}

	credsBody, err := jwt.FormatUserConfig(spec.Cred.UserJWT, spec.Cred.UserSeed)
	if err != nil {
		return nil, fmt.Errorf("k8s: format creds: %w", err)
	}
	ns := b.namespace()
	labels := map[string]string{managedByKey: managedByValue}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Data:       map[string][]byte{"nats.creds": credsBody},
	}
	if _, err := b.Client.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return nil, fmt.Errorf("k8s: create secret: %w", err)
	}

	// The watch is established BEFORE the pod exists: a fast-crashing
	// workload can reach its terminal state in the gap, and status history
	// is not replayed to a late watcher (research D4).
	w, err := b.Client.CoreV1().Pods(ns).Watch(context.Background(), metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", name).String(),
	})
	if err != nil {
		_ = b.Client.CoreV1().Secrets(ns).Delete(ctx, name, metav1.DeleteOptions{})
		return nil, fmt.Errorf("k8s: watch pod: %w", err)
	}

	grace := int64(stopGrace.Seconds())
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Spec: corev1.PodSpec{
			// Supervision stays with the runner: the cluster never restarts
			// a workload the runner believes is dead (FR-008).
			RestartPolicy:                 corev1.RestartPolicyNever,
			TerminationGracePeriodSeconds: &grace,
			Volumes: []corev1.Volume{
				{Name: "scratch", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "creds", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: name}}},
			},
			Containers: []corev1.Container{{
				Name:       "workload",
				Image:      ref,
				Command:    append([]string{guestWorkload}, spec.Args...),
				WorkingDir: guestScratch,
				VolumeMounts: []corev1.VolumeMount{
					{Name: "scratch", MountPath: guestScratch},
					{Name: "creds", MountPath: guestCredsDir, ReadOnly: true},
				},
				Env: podEnv(spec, b.HostAlias),
			}},
		},
	}
	if _, err := b.Client.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		w.Stop()
		_ = b.Client.CoreV1().Secrets(ns).Delete(ctx, name, metav1.DeleteOptions{})
		return nil, fmt.Errorf("k8s: create pod: %w", err)
	}

	h := &handle{b: b, name: name, ns: ns, done: make(chan backend.ExitStatus, 1)}
	go h.supervise(w)
	return h, nil
}

// podEnv is the workload-env contract — native's variable names, values
// adapted to the pod (contracts/workload-env.md; msb parity).
func podEnv(spec backend.LaunchSpec, alias string) []corev1.EnvVar {
	servers := spec.Cred.NatsServers
	if alias != "" {
		servers = natsurl.Rewrite(servers, alias)
	}
	return []corev1.EnvVar{
		{Name: native.EnvNatsServers, Value: strings.Join(servers, ",")},
		{Name: native.EnvCredsFile, Value: guestCredsPath},
		{Name: native.EnvRealm, Value: spec.Realm},
		{Name: native.EnvPersona, Value: spec.Cred.Persona},
		{Name: native.EnvTopic, Value: spec.Topic},
	}
}

type handle struct {
	b    *Backend
	name string
	ns   string

	done chan backend.ExitStatus

	once   sync.Once
	status backend.ExitStatus
}

// supervise consumes the watch to a terminal state, capturing the container
// termination state on EVERY update (after deletion it is gone — research
// D4), re-establishing the watch if the server closes it early, then reaps
// and delivers the mapped status.
func (h *handle) supervise(w watch.Interface) {
	ctx := context.Background()
	var last *corev1.ContainerStateTerminated

	for {
		terminal := false
		for ev := range w.ResultChan() {
			p, ok := ev.Object.(*corev1.Pod)
			if ok && p.Name == h.name {
				capture(&last, p)
			}
			if ev.Type == watch.Deleted {
				terminal = true
				break
			}
			if ok && p.Name == h.name &&
				(p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed) {
				terminal = true
				break
			}
		}
		w.Stop()
		if terminal {
			break
		}
		// Channel closed without a terminal state (server-side watch
		// timeout). Check current reality, then watch again. A Get error
		// means the pod is gone (treat as deleted); a Watch error means it
		// cannot be observed any further — reap with what was captured.
		p, err := h.b.Client.CoreV1().Pods(h.ns).Get(ctx, h.name, metav1.GetOptions{})
		if err != nil {
			break
		}
		capture(&last, p)
		if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
			break
		}
		nw, err := h.b.Client.CoreV1().Pods(h.ns).Watch(ctx, metav1.ListOptions{
			FieldSelector: fields.OneTermEqualSelector("metadata.name", h.name).String(),
		})
		if err != nil {
			break
		}
		w = nw
	}

	h.reap(ctx)
	h.done <- mapExit(last)
}

func capture(last **corev1.ContainerStateTerminated, p *corev1.Pod) {
	for _, cst := range p.Status.ContainerStatuses {
		if cst.State.Terminated != nil {
			t := *cst.State.Terminated
			*last = &t
		}
	}
}

// reap removes everything Start created on the cluster. Idempotent; grace 0
// because the workload is already terminal (or deleting) by the time it runs.
// The per-run image stays in the registry under operator retention (D2).
func (h *handle) reap(ctx context.Context) {
	zero := int64(0)
	_ = h.b.Client.CoreV1().Pods(h.ns).Delete(ctx, h.name, metav1.DeleteOptions{GracePeriodSeconds: &zero})
	_ = h.b.Client.CoreV1().Secrets(h.ns).Delete(ctx, h.name, metav1.DeleteOptions{})
}

// Wait blocks until the pod reaches a terminal state, everything is reaped,
// and returns the mapped exit status. Safe to call more than once.
func (h *handle) Wait() backend.ExitStatus {
	h.once.Do(func() { h.status = <-h.done })
	return h.status
}

// Stop deletes the pod with a grace period derived from ctx (capped at the
// stop grace): Kubernetes sends TERM at delete and KILL after the grace —
// the native SIGTERM→SIGKILL escalation in cluster vocabulary. Stop
// publishes nothing; the runner owns the terminal op.
func (h *handle) Stop(ctx context.Context) error {
	grace := int64(stopGrace.Seconds())
	if dl, ok := ctx.Deadline(); ok {
		if s := int64(time.Until(dl).Seconds()); s >= 0 && s < grace {
			grace = s
		}
	}
	err := h.b.Client.CoreV1().Pods(h.ns).Delete(ctx, h.name, metav1.DeleteOptions{GracePeriodSeconds: &grace})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("k8s: delete pod: %w", err)
	}
	return nil
}

// mapExit maps the last observed termination state to a backend.ExitStatus.
// Kubernetes does not populate the Signal field (research, measured): a
// signal death arrives as exitCode 128+n, so the signal is inferred — and a
// workload that literally exits >128 is indistinguishable (named
// limitation, design 0002 §5). Deleted before any termination state was
// observable maps to the seam's uncoded failure.
func mapExit(t *corev1.ContainerStateTerminated) backend.ExitStatus {
	if t == nil {
		return backend.ExitStatus{Code: -1}
	}
	if t.Signal != 0 {
		return backend.ExitStatus{Signal: syscall.Signal(t.Signal).String()}
	}
	if t.ExitCode > 128 {
		return backend.ExitStatus{Signal: syscall.Signal(t.ExitCode - 128).String()}
	}
	return backend.ExitStatus{Code: int(t.ExitCode)}
}

// podName derives the pod's (and Secret's, and image tag's) name from the
// scratch dir's base — the work item id (msb's sandboxName convention) —
// sanitized to RFC 1123: lowercase alphanumerics and '-', bounded, trimmed.
func podName(scratchDir string) string {
	base := strings.ToLower(filepath.Base(scratchDir))
	mapped := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		default:
			return '-'
		}
	}, base)
	const maxBase = 50
	if len(mapped) > maxBase {
		mapped = mapped[:maxBase]
	}
	return podPrefix + strings.Trim(mapped, "-")
}

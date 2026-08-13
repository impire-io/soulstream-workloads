package k8s

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/impire-io/soulstream-workloads/backend"
	"github.com/impire-io/soulstream-workloads/backend/native"
	"github.com/impire-io/soulstream-workloads/minter"
)

const testNS = "test-ns"

// stubPublisher is the hermetic ImagePublisher: records what it was asked to
// publish and returns a fixed digest-pinned ref (the real one is covered by
// image_test.go against the in-process registry).
type stubPublisher struct {
	mu       sync.Mutex
	calls    int
	artifact []byte
	tag      string
	fail     bool
}

func (s *stubPublisher) Publish(_ context.Context, artifact []byte, tag string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = 1 + s.calls
	s.artifact = append([]byte(nil), artifact...)
	s.tag = tag
	if s.fail {
		return "", context.DeadlineExceeded
	}
	return "reg.test/soulstream-workloads/workloads@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", nil
}

// testCred mints a real scoped credential from a throwaway account — no
// server involved; Mint is pure signing (the msb test pattern).
func testCred(t *testing.T, servers ...string) minter.PersonaScopedCredential {
	t.Helper()
	if len(servers) == 0 {
		servers = []string{"nats://127.0.0.1:4222"}
	}
	acc, err := nkeys.CreateAccount()
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	pub, _ := acc.PublicKey()
	seed, _ := acc.Seed()
	m, err := minter.NewSigningKeyMinter(seed, pub, servers)
	if err != nil {
		t.Fatalf("minter: %v", err)
	}
	cred, err := m.Mint(minter.Scope{Persona: "researcher", Topic: "planning-ab12"}, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return cred
}

func elfArtifact(t *testing.T) string {
	t.Helper()
	return writeArtifact(t, append([]byte{0x7f, 'E', 'L', 'F'}, []byte("fake linux binary")...))
}

func writeArtifact(t *testing.T, content []byte) string {
	t.Helper()
	path := t.TempDir() + "/agent-echo"
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	return path
}

func testSpec(t *testing.T, artifact string, cred minter.PersonaScopedCredential) backend.LaunchSpec {
	t.Helper()
	return backend.LaunchSpec{
		Artifact:   artifact,
		Args:       []string{"--flag", "value"},
		Cred:       cred,
		Realm:      "test-realm",
		Topic:      "planning-ab12",
		ScratchDir: "/tmp/scratch/Item.42",
	}
}

func newTestBackend(cs *fake.Clientset, stub *stubPublisher) *Backend {
	return &Backend{
		Client:    cs,
		Namespace: testNS,
		Registry:  "reg.test/soulstream-workloads",
		HostAlias: "node.test.internal",
		Images:    stub,
	}
}

// finishPod drives the fake pod to a terminal state the way the kubelet
// would: a status update carrying phase + termination state (T006 harness).
func finishPod(t *testing.T, cs *fake.Clientset, name string, phase corev1.PodPhase, term *corev1.ContainerStateTerminated) {
	t.Helper()
	p, err := cs.CoreV1().Pods(testNS).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pod: %v", err)
	}
	p.Status.Phase = phase
	if term != nil {
		p.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name:  "workload",
			State: corev1.ContainerState{Terminated: term},
		}}
	}
	if _, err := cs.CoreV1().Pods(testNS).Update(context.Background(), p, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update pod status: %v", err)
	}
}

func requireClean(t *testing.T, cs *fake.Clientset) {
	t.Helper()
	pods, _ := cs.CoreV1().Pods(testNS).List(context.Background(), metav1.ListOptions{})
	if len(pods.Items) != 0 {
		t.Fatalf("pods remain after end of life: %d", len(pods.Items))
	}
	secrets, _ := cs.CoreV1().Secrets(testNS).List(context.Background(), metav1.ListOptions{})
	if len(secrets.Items) != 0 {
		t.Fatalf("secrets remain after end of life: %d", len(secrets.Items))
	}
}

func TestStartShapesPodAndSecret(t *testing.T) {
	cs := fake.NewClientset()
	stub := &stubPublisher{}
	b := newTestBackend(cs, stub)
	cred := testCred(t) // loopback server → must be rewritten to the alias
	spec := testSpec(t, elfArtifact(t), cred)

	h, err := b.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	const wantName = "soulstream-workloads-item-42" // sanitized RFC 1123 from Item.42

	// The Secret: creds-file bytes under the pod's name, labeled.
	sec, err := cs.CoreV1().Secrets(testNS).Get(context.Background(), wantName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("secret: %v", err)
	}
	wantCreds, _ := jwt.FormatUserConfig(cred.UserJWT, cred.UserSeed)
	if string(sec.Data["nats.creds"]) != string(wantCreds) {
		t.Error("secret does not carry the formatted creds file")
	}
	if sec.Labels[managedByKey] != managedByValue {
		t.Error("secret missing the managed-by label")
	}

	p, err := cs.CoreV1().Pods(testNS).Get(context.Background(), wantName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("pod: %v", err)
	}
	if p.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("restartPolicy = %s, want Never (FR-008)", p.Spec.RestartPolicy)
	}
	if p.Spec.TerminationGracePeriodSeconds == nil || *p.Spec.TerminationGracePeriodSeconds != 5 {
		t.Error("termination grace != stop grace")
	}
	if p.Labels[managedByKey] != managedByValue {
		t.Error("pod missing the managed-by label")
	}
	// Backend, not scheduler (FR-010): no placement opinions in the spec.
	if p.Spec.NodeSelector != nil || p.Spec.Affinity != nil || p.Spec.NodeName != "" {
		t.Error("pod spec carries placement decisions")
	}
	if len(p.Spec.Containers) != 1 {
		t.Fatalf("containers = %d, want 1", len(p.Spec.Containers))
	}
	c := p.Spec.Containers[0]
	if c.Image != "reg.test/soulstream-workloads/workloads@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Errorf("image = %q, want the digest-pinned publisher ref", c.Image)
	}
	if strings.Join(c.Command, " ") != guestWorkload+" --flag value" {
		t.Errorf("command = %v", c.Command)
	}
	if c.WorkingDir != guestScratch {
		t.Errorf("workdir = %q, want %s (native parity)", c.WorkingDir, guestScratch)
	}

	// Env: exactly the workload contract — nothing inherited, servers
	// rewritten to the alias, creds path in-pod.
	env := map[string]string{}
	for _, e := range c.Env {
		env[e.Name] = e.Value
	}
	want := map[string]string{
		native.EnvNatsServers: "nats://node.test.internal:4222",
		native.EnvCredsFile:   guestCredsPath,
		native.EnvRealm:       "test-realm",
		native.EnvPersona:     "researcher",
		native.EnvTopic:       "planning-ab12",
	}
	if len(env) != len(want) {
		t.Errorf("env has %d vars, want exactly %d (clean env, constitution II)", len(env), len(want))
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("env %s = %q, want %q", k, env[k], v)
		}
	}

	// The publisher saw the artifact bytes and the pod name as tag.
	if stub.calls != 1 || stub.tag != wantName {
		t.Errorf("publisher calls=%d tag=%q", stub.calls, stub.tag)
	}

	// Clean exit → Code 0, everything reaped.
	finishPod(t, cs, wantName, corev1.PodSucceeded,
		&corev1.ContainerStateTerminated{ExitCode: 0, Reason: "Completed"})
	if st := h.Wait(); st.Code != 0 || st.Signal != "" {
		t.Fatalf("Wait = %+v, want clean 0", st)
	}
	if st := h.Wait(); st.Code != 0 { // idempotent
		t.Fatalf("second Wait = %+v", st)
	}
	requireClean(t, cs)
}

func TestNonELFArtifactRefused(t *testing.T) {
	cs := fake.NewClientset()
	stub := &stubPublisher{}
	b := newTestBackend(cs, stub)
	spec := testSpec(t, writeArtifact(t, []byte("#!/bin/sh\necho mach-o pretender\n")), testCred(t))

	if _, err := b.Start(context.Background(), spec); err == nil || !strings.Contains(err.Error(), "ELF") {
		t.Fatalf("want pre-launch ELF refusal, got %v", err)
	}
	if stub.calls != 0 {
		t.Error("publisher was called for a refused artifact")
	}
	requireClean(t, cs)
}

func TestLoopbackWithoutAliasFailsLoud(t *testing.T) {
	cs := fake.NewClientset()
	stub := &stubPublisher{}
	b := newTestBackend(cs, stub)
	b.HostAlias = ""
	spec := testSpec(t, elfArtifact(t), testCred(t)) // loopback cred

	if _, err := b.Start(context.Background(), spec); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("want loopback-without-alias refusal, got %v", err)
	}
	if stub.calls != 0 {
		t.Error("publisher was called before the loopback guard")
	}
	requireClean(t, cs)
}

func TestRoutableServersPassThroughUnrewritten(t *testing.T) {
	cs := fake.NewClientset()
	b := newTestBackend(cs, &stubPublisher{})
	b.HostAlias = "" // routable realm needs no alias
	cred := testCred(t, "tls://connect.example.com")
	spec := testSpec(t, elfArtifact(t), cred)

	h, err := b.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	p, _ := cs.CoreV1().Pods(testNS).Get(context.Background(), "soulstream-workloads-item-42", metav1.GetOptions{})
	for _, e := range p.Spec.Containers[0].Env {
		if e.Name == native.EnvNatsServers && e.Value != "tls://connect.example.com" {
			t.Errorf("routable server rewritten: %q", e.Value)
		}
	}
	finishPod(t, cs, "soulstream-workloads-item-42", corev1.PodSucceeded,
		&corev1.ContainerStateTerminated{ExitCode: 0})
	h.Wait()
	requireClean(t, cs)
}

func TestStartFailureRollsBack(t *testing.T) {
	cs := fake.NewClientset()
	cs.PrependReactor("create", "pods", func(clientgotesting.Action) (bool, runtime.Object, error) {
		return true, nil, context.DeadlineExceeded
	})
	b := newTestBackend(cs, &stubPublisher{})
	if _, err := b.Start(context.Background(), testSpec(t, elfArtifact(t), testCred(t))); err == nil {
		t.Fatal("want start failure")
	}
	requireClean(t, cs) // the Secret must not survive the failed launch
}

func TestCrashMapsExitCode(t *testing.T) {
	cs := fake.NewClientset()
	b := newTestBackend(cs, &stubPublisher{})
	h, err := b.Start(context.Background(), testSpec(t, elfArtifact(t), testCred(t)))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	finishPod(t, cs, "soulstream-workloads-item-42", corev1.PodFailed,
		&corev1.ContainerStateTerminated{ExitCode: 3, Reason: "Error"})
	if st := h.Wait(); st.Code != 3 || st.Signal != "" {
		t.Fatalf("Wait = %+v, want Code 3", st)
	}
	requireClean(t, cs)
}

func TestStopDerivesGraceFromContext(t *testing.T) {
	cs := fake.NewClientset()
	b := newTestBackend(cs, &stubPublisher{})
	h, err := b.Start(context.Background(), testSpec(t, elfArtifact(t), testCred(t)))
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// Terminal state first (the kubelet would produce 137 after the grace),
	// then Stop with a 2s deadline: the issued grace must be ≤ 2.
	finishPod(t, cs, "soulstream-workloads-item-42", corev1.PodFailed,
		&corev1.ContainerStateTerminated{ExitCode: 137, Reason: "Error"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
	var got *int64
	for _, a := range cs.Actions() {
		if d, ok := a.(clientgotesting.DeleteActionImpl); ok && d.GetResource().Resource == "pods" {
			got = d.DeleteOptions.GracePeriodSeconds
		}
	}
	if got == nil || *got > 2 || *got < 0 {
		t.Fatalf("delete grace = %v, want derived from ctx (≤2)", got)
	}
	// 137 = 128+9: the inferred-signal named limitation, mapped as a signal.
	if st := h.Wait(); st.Signal != "killed" {
		t.Fatalf("Wait = %+v, want inferred signal \"killed\"", st)
	}
	requireClean(t, cs)
}

// TestOutOfBandDeletion is analysis T013: the cluster (not the runner)
// deletes the pod before any termination state is observable — Wait must
// return the uncoded failure and still reap the Secret.
func TestOutOfBandDeletion(t *testing.T) {
	cs := fake.NewClientset()
	b := newTestBackend(cs, &stubPublisher{})
	h, err := b.Start(context.Background(), testSpec(t, elfArtifact(t), testCred(t)))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := cs.CoreV1().Pods(testNS).Delete(context.Background(), "soulstream-workloads-item-42", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("out-of-band delete: %v", err)
	}
	if st := h.Wait(); st.Code != -1 || st.Signal != "" {
		t.Fatalf("Wait = %+v, want the uncoded failure (Code -1)", st)
	}
	requireClean(t, cs)
}

func TestMapExit(t *testing.T) {
	cases := []struct {
		name string
		in   *corev1.ContainerStateTerminated
		want backend.ExitStatus
	}{
		{"nil (deleted unobserved)", nil, backend.ExitStatus{Code: -1}},
		{"clean", &corev1.ContainerStateTerminated{ExitCode: 0}, backend.ExitStatus{Code: 0}},
		{"crash", &corev1.ContainerStateTerminated{ExitCode: 3}, backend.ExitStatus{Code: 3}},
		{"explicit signal field", &corev1.ContainerStateTerminated{ExitCode: 137, Signal: 9}, backend.ExitStatus{Signal: "killed"}},
		{"inferred SIGKILL (128+9)", &corev1.ContainerStateTerminated{ExitCode: 137}, backend.ExitStatus{Signal: "killed"}},
		{"inferred SIGTERM (128+15)", &corev1.ContainerStateTerminated{ExitCode: 143}, backend.ExitStatus{Signal: "terminated"}},
	}
	for _, c := range cases {
		if got := mapExit(c.in); got != c.want {
			t.Errorf("%s: mapExit = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestPodName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/tmp/scratch/item-42", "soulstream-workloads-item-42"},
		{"/tmp/scratch/It.em:42", "soulstream-workloads-it-em-42"},
		{"/tmp/scratch/--weird--", "soulstream-workloads-weird"},
		{"/tmp/scratch/" + strings.Repeat("A", 150), "soulstream-workloads-" + strings.Repeat("a", 50)},
	}
	for _, c := range cases {
		if got := podName(c.in); got != c.want {
			t.Errorf("podName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

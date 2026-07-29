//go:build k8s_e2e

// The M2.1 proof, run by `make test-k8s` against a local kind cluster plus a
// local OCI registry (`scripts/kind-registry.sh up` — research D7): the SAME
// declarations that run natively run as Kubernetes pods, with the same op
// mapping on the topic (constitution III), the artifact riding a per-run OCI
// image through the real push→pull path, and the credential enforced from
// inside the pod. Build-tagged, never skipped: without the tag these tests
// do not exist; with it they must pass.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/impire-io/soulstream/realm"
	"github.com/impire-io/soulstream/topic"

	"github.com/impire-io/soulrealm/backend"
	"github.com/impire-io/soulrealm/backend/k8s"
	"github.com/impire-io/soulrealm/backend/native"
	"github.com/impire-io/soulrealm/declaration"
	"github.com/impire-io/soulrealm/internal/natstest"
	"github.com/impire-io/soulrealm/minter"
	"github.com/impire-io/soulrealm/runner"
)

const (
	k8sNamespace = "soulrealm-e2e"
	managedBy    = "app.kubernetes.io/managed-by=soulrealm"
)

func k8sEnv(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// k8sTestBackend builds the backend against the script-provisioned cluster
// and registry (overridable via SOULREALM_K8S_E2E_* for other environments).
func k8sTestBackend(t *testing.T) *k8s.Backend {
	t.Helper()
	kctx := k8sEnv("SOULREALM_K8S_E2E_CONTEXT", "kind-soulrealm-k8s")
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules, &clientcmd.ConfigOverrides{CurrentContext: kctx}).ClientConfig()
	if err != nil {
		t.Fatalf("kubeconfig (%s — run scripts/kind-registry.sh up): %v", kctx, err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	_, _ = cs.CoreV1().Namespaces().Create(context.Background(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: k8sNamespace}}, metav1.CreateOptions{})
	return &k8s.Backend{
		Client:    cs,
		Namespace: k8sNamespace,
		Registry:  k8sEnv("SOULREALM_K8S_E2E_REGISTRY", "localhost:5001/soulrealm"),
		HostAlias: detectHostAlias(t),
	}
}

// detectHostAlias finds the address pods reach the host at: an env override,
// or Docker's host mapping as seen from the kind node.
func detectHostAlias(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("SOULREALM_K8S_E2E_HOST_ALIAS"); v != "" {
		return v
	}
	node := k8sEnv("SOULREALM_K8S_E2E_NODE", "soulrealm-k8s-control-plane")
	out, err := exec.Command("docker", "exec", node, "getent", "hosts", "host.docker.internal").Output()
	if err != nil {
		t.Fatalf("detect host alias via %s (set SOULREALM_K8S_E2E_HOST_ALIAS): %v", node, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		t.Fatal("host.docker.internal did not resolve on the kind node")
	}
	return fields[0]
}

// startNatsForPods binds the in-process JetStream server to 0.0.0.0 (T015)
// and returns the loopback URL the runner and minter use — the backend's
// loopback→alias rewrite is exactly what carries pods to it.
func startNatsForPods(t *testing.T) (loopURL string, cleanup func()) {
	t.Helper()
	url, cleanup := natstest.StartJetStream(t, natstest.WithBindAddress("0.0.0.0"))
	u, err := neturl.Parse(url)
	if err != nil {
		t.Fatalf("parse nats url %q: %v", url, err)
	}
	return "nats://127.0.0.1:" + u.Port(), cleanup
}

// requireCleanCluster is the zero-leftovers clause (SC-003): no managed pod
// or Secret survives its workload. Deletion is issued before Wait returns;
// a short window lets the API server finish removing terminating objects.
func requireCleanCluster(t *testing.T, b *k8s.Backend) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		pods, err := b.Client.CoreV1().Pods(k8sNamespace).List(context.Background(),
			metav1.ListOptions{LabelSelector: managedBy})
		if err != nil {
			t.Fatalf("list pods: %v", err)
		}
		secrets, err := b.Client.CoreV1().Secrets(k8sNamespace).List(context.Background(),
			metav1.ListOptions{LabelSelector: managedBy})
		if err != nil {
			t.Fatalf("list secrets: %v", err)
		}
		if len(pods.Items) == 0 && len(secrets.Items) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("leftovers on the cluster: %d pods, %d secrets", len(pods.Items), len(secrets.Items))
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// TestK8sLaunchAgentEndToEnd is SC-001 with the zero-diff clause enacted:
// ONE declaration value, marshalled and compared byte-for-byte across the
// native control run and the pod run; the node swaps the artifact build at
// the declared path between runs (host build for native, linux build for the
// pod — node-side provisioning, the M1.3 convention).
func TestK8sLaunchAgentEndToEnd(t *testing.T) {
	artifactPath := filepath.Join(t.TempDir(), "agent-echo")
	hostBuild := buildCmd(t, "github.com/impire-io/soulrealm/cmd/agent-echo")
	linuxBuild := buildCmdLinux(t, "github.com/impire-io/soulrealm/cmd/agent-echo")
	provision := func(src string) {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read %s: %v", src, err)
		}
		if err := os.WriteFile(artifactPath, data, 0o755); err != nil {
			t.Fatalf("provision artifact: %v", err)
		}
	}

	url, shutdown := startNatsForPods(t)
	defer shutdown()
	ctx := context.Background()

	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	runnerClient, err := realm.NewClient(ctx, nc, realm.Config{Realm: "test-realm", Persona: "soulrealm-runner"})
	if err != nil {
		t.Fatalf("runner client: %v", err)
	}
	if _, err := runnerClient.Provision(ctx); err != nil {
		t.Fatalf("provision realm: %v", err)
	}
	h, err := topic.StartTopic(ctx, runnerClient, topic.StartTopicInput{Name: "planning"})
	if err != nil {
		t.Fatalf("start topic: %v", err)
	}
	topicPath := h.Path()
	m := e2eMinter(t, url)

	d := declaration.Declaration{
		Role:      declaration.RoleAgent,
		Lifecycle: declaration.LifecycleService,
		Persona:   "researcher",
		Topic:     topicPath,
		Artifact:  "file://" + artifactPath,
	}
	declNative, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Native control run — the comparison arm.
	provision(hostBuild)
	rn := &runner.Runner{Minter: m, Backend: native.New(), Realm: "test-realm", CredTTL: time.Hour, ScratchRoot: t.TempDir()}
	if err := rn.Run(ctx, topic.Open(runnerClient, topicPath), d); err != nil {
		t.Fatalf("Run under native: %v", err)
	}

	// Same declaration value, byte-for-byte (US1 acceptance 2 / SC-001).
	declK8s, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(declNative, declK8s) {
		t.Fatalf("declaration changed between backends:\n%s\n%s", declNative, declK8s)
	}

	// The pod run: node provisions the linux build at the same declared path.
	provision(linuxBuild)
	b := k8sTestBackend(t)
	rk := &runner.Runner{Minter: m, Backend: b, Realm: "test-realm", CredTTL: time.Hour, ScratchRoot: t.TempDir()}
	if err := rk.Run(ctx, topic.Open(runnerClient, topicPath), d); err != nil {
		t.Fatalf("Run under k8s: %v", err)
	}

	mt, err := topic.Open(runnerClient, topicPath).Materialise(ctx)
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}
	var turns, done int
	for _, c := range mt.Contributions {
		if c.Author == "researcher" && strings.Contains(c.Body, "researcher") {
			turns++
		}
	}
	for _, w := range mt.WorkItems {
		if w.Status == topic.WorkDone && w.Author == "soulrealm-runner" {
			done++
		}
	}
	if turns != 2 {
		t.Fatalf("SC-001: turns by researcher = %d, want 2 (native + k8s); contributions=%+v", turns, mt.Contributions)
	}
	if done != 2 {
		t.Fatalf("SC-001: done work items = %d, want 2; workitems=%+v", done, mt.WorkItems)
	}
	requireCleanCluster(t, b)
}

// TestK8sAgentCallsToolEndToEnd is SC-002: the M1.2 tool scenario with the
// tool INSIDE a pod — discovery by name, uppercase round trip, stop →
// work.done, everything reaped.
func TestK8sAgentCallsToolEndToEnd(t *testing.T) {
	toolPath := buildCmdLinux(t, "github.com/impire-io/soulrealm/cmd/tool-upper")

	url, shutdown := startNatsForPods(t)
	defer shutdown()
	ctx := context.Background()

	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	runnerClient, err := realm.NewClient(ctx, nc, realm.Config{Realm: "test-realm", Persona: "soulrealm-runner"})
	if err != nil {
		t.Fatalf("runner client: %v", err)
	}
	if _, err := runnerClient.Provision(ctx); err != nil {
		t.Fatalf("provision realm: %v", err)
	}
	h, err := topic.StartTopic(ctx, runnerClient, topic.StartTopicInput{Name: "tools"})
	if err != nil {
		t.Fatalf("start topic: %v", err)
	}
	topicPath := h.Path()
	m := e2eMinter(t, url)

	toolDecl := declaration.Declaration{
		Role:      declaration.RoleTool,
		Lifecycle: declaration.LifecycleService,
		Persona:   "uppercase",
		Topic:     topicPath,
		Artifact:  "file://" + toolPath,
	}
	b := k8sTestBackend(t)
	r := &runner.Runner{Minter: m, Backend: b, Realm: "test-realm", CredTTL: time.Hour, ScratchRoot: t.TempDir()}
	rw, err := r.Launch(ctx, topic.Open(runnerClient, topicPath), toolDecl)
	if err != nil {
		t.Fatalf("launch tool: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			_ = rw.Stop(context.Background())
		}
	})

	callerCred, err := m.Mint(minter.Scope{Role: declaration.RoleAgent, Persona: "researcher", Topic: topicPath}, time.Hour)
	if err != nil {
		t.Fatalf("mint caller: %v", err)
	}
	cnc, err := nats.Connect(url, nats.UserCredentials(writeCreds(t, callerCred)))
	if err != nil {
		t.Fatalf("caller connect: %v", err)
	}
	defer cnc.Close()

	// Discovery window ≥ 60s: covers image assembly, push, and a cold pull.
	subject := minter.ServiceSubject("uppercase")
	var reply *nats.Msg
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		reply, err = cnc.Request(subject, []byte("hi"), 500*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("SC-002: calling the pod tool failed: %v", err)
	}
	if string(reply.Data) != "HI" {
		t.Fatalf("SC-002: reply = %q, want HI", reply.Data)
	}

	if err := rw.Stop(ctx); err != nil {
		t.Fatalf("stop tool: %v", err)
	}
	stopped = true

	mt, err := topic.Open(runnerClient, topicPath).Materialise(ctx)
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}
	var doneFound bool
	for _, w := range mt.WorkItems {
		if w.Status == topic.WorkDone && w.Author == "soulrealm-runner" {
			doneFound = true
		}
	}
	if !doneFound {
		t.Fatalf("SC-002: no completed tool work item; workitems=%+v", mt.WorkItems)
	}
	requireCleanCluster(t, b)
}

// TestK8sCrashAbandons is SC-003: a workload dying nonzero inside its pod
// ends as work.abandon and leaves nothing on the cluster.
func TestK8sCrashAbandons(t *testing.T) {
	crash := buildInline(t, "crash", "package main\n\nimport \"os\"\n\nfunc main() { os.Exit(3) }\n", true)

	url, shutdown := startNatsForPods(t)
	defer shutdown()
	ctx := context.Background()

	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	runnerClient, err := realm.NewClient(ctx, nc, realm.Config{Realm: "test-realm", Persona: "soulrealm-runner"})
	if err != nil {
		t.Fatalf("runner client: %v", err)
	}
	if _, err := runnerClient.Provision(ctx); err != nil {
		t.Fatalf("provision realm: %v", err)
	}
	h, err := topic.StartTopic(ctx, runnerClient, topic.StartTopicInput{Name: "crashes"})
	if err != nil {
		t.Fatalf("start topic: %v", err)
	}
	topicPath := h.Path()

	d := declaration.Declaration{
		Role:      declaration.RoleAgent,
		Lifecycle: declaration.LifecycleService,
		Persona:   "crasher",
		Topic:     topicPath,
		Artifact:  "file://" + crash,
	}
	b := k8sTestBackend(t)
	r := &runner.Runner{Minter: e2eMinter(t, url), Backend: b, Realm: "test-realm", CredTTL: time.Hour, ScratchRoot: t.TempDir()}
	if err := r.Run(ctx, topic.Open(runnerClient, topicPath), d); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mt, err := topic.Open(runnerClient, topicPath).Materialise(ctx)
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}
	var abandoned bool
	for _, w := range mt.WorkItems {
		if w.Author != "soulrealm-runner" {
			continue
		}
		for _, ev := range w.Timeline {
			if ev.Kind == "abandon" && !ev.Void && ev.Author == "soulrealm-runner" {
				abandoned = true
			}
		}
	}
	if !abandoned {
		t.Fatalf("SC-003: no abandon event for the crashed workload; workitems=%+v", mt.WorkItems)
	}
	requireCleanCluster(t, b)
}

// TestK8sOutOfBandDeletion is US3's interference edge: the CLUSTER (not the
// runner) deletes the running pod — the run must still close as
// work.abandon, and no second copy of the workload may ever appear.
func TestK8sOutOfBandDeletion(t *testing.T) {
	sleeper := buildInline(t, "sleeper",
		"package main\n\nimport \"time\"\n\nfunc main() { time.Sleep(300 * time.Second) }\n", true)

	url, shutdown := startNatsForPods(t)
	defer shutdown()
	ctx := context.Background()

	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	runnerClient, err := realm.NewClient(ctx, nc, realm.Config{Realm: "test-realm", Persona: "soulrealm-runner"})
	if err != nil {
		t.Fatalf("runner client: %v", err)
	}
	if _, err := runnerClient.Provision(ctx); err != nil {
		t.Fatalf("provision realm: %v", err)
	}
	h, err := topic.StartTopic(ctx, runnerClient, topic.StartTopicInput{Name: "interference"})
	if err != nil {
		t.Fatalf("start topic: %v", err)
	}
	topicPath := h.Path()

	d := declaration.Declaration{
		Role:      declaration.RoleAgent,
		Lifecycle: declaration.LifecycleService,
		Persona:   "sleeper",
		Topic:     topicPath,
		Artifact:  "file://" + sleeper,
	}
	b := k8sTestBackend(t)
	r := &runner.Runner{Minter: e2eMinter(t, url), Backend: b, Realm: "test-realm", CredTTL: time.Hour, ScratchRoot: t.TempDir()}
	rw, err := r.Launch(ctx, topic.Open(runnerClient, topicPath), d)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}

	// Wait for the pod to run, then delete it out-of-band.
	var podName string
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		pods, err := b.Client.CoreV1().Pods(k8sNamespace).List(ctx, metav1.ListOptions{LabelSelector: managedBy})
		if err == nil && len(pods.Items) == 1 && pods.Items[0].Status.Phase == corev1.PodRunning {
			podName = pods.Items[0].Name
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if podName == "" {
		t.Fatal("workload pod never reached Running")
	}
	zero := int64(0)
	if err := b.Client.CoreV1().Pods(k8sNamespace).Delete(ctx, podName, metav1.DeleteOptions{GracePeriodSeconds: &zero}); err != nil {
		t.Fatalf("out-of-band delete: %v", err)
	}

	// The runner observes the death and closes the lifecycle.
	if err := rw.Wait(); err != nil {
		t.Fatalf("wait after interference: %v", err)
	}
	mt, err := topic.Open(runnerClient, topicPath).Materialise(ctx)
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}
	var abandoned bool
	for _, w := range mt.WorkItems {
		for _, ev := range w.Timeline {
			if ev.Kind == "abandon" && !ev.Void && ev.Author == "soulrealm-runner" {
				abandoned = true
			}
		}
	}
	if !abandoned {
		t.Fatalf("no abandon after out-of-band deletion; workitems=%+v", mt.WorkItems)
	}
	requireCleanCluster(t, b) // and no resurrected copy
}

// TestK8sScopeEnforcedFromPod is SC-004: against an operator-mode server
// that ENFORCES user JWT permissions, the scope probe succeeds in-scope and
// is denied out-of-scope — from inside the pod, credential via Secret. A
// native control arm separates credential faults from pod faults.
func TestK8sScopeEnforcedFromPod(t *testing.T) {
	op := natstest.StartOperator(t, natstest.WithBindAddress("0.0.0.0"))
	defer op.Shutdown()
	u, err := neturl.Parse(op.URL)
	if err != nil {
		t.Fatalf("parse operator url: %v", err)
	}
	loopURL := "nats://127.0.0.1:" + u.Port()

	m, err := minter.NewSigningKeyMinter(op.AccountSigningSeed, op.RootAccountKey, []string{loopURL})
	if err != nil {
		t.Fatalf("minter: %v", err)
	}
	cred, err := m.Mint(minter.Scope{Role: declaration.RoleAgent, Persona: "probe", Topic: "t-scope"}, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	ctx := context.Background()
	spec := func(artifact, scratch string) backend.LaunchSpec {
		return backend.LaunchSpec{
			Artifact:   artifact,
			Cred:       cred,
			Realm:      "test-realm",
			Topic:      "t-scope",
			ScratchDir: scratch,
		}
	}

	// Control arm: the identical probe, natively (no rewrite, loopback).
	hostProbe := buildCmd(t, "github.com/impire-io/soulrealm/cmd/scope-probe")
	nh, err := native.New().Start(ctx, spec(hostProbe, filepath.Join(t.TempDir(), "n")))
	if err != nil {
		t.Fatalf("native start: %v", err)
	}
	if st := nh.Wait(); st.Code != 0 || st.Signal != "" {
		t.Fatalf("SC-004 control: native probe = %+v, want clean 0", st)
	}

	// The bar: the same probe from inside a pod, credential via Secret, the
	// operator server reached through the host alias.
	linuxProbe := buildCmdLinux(t, "github.com/impire-io/soulrealm/cmd/scope-probe")
	b := k8sTestBackend(t)
	kh, err := b.Start(ctx, spec(linuxProbe, filepath.Join(t.TempDir(), "probe-scope")))
	if err != nil {
		t.Fatalf("k8s start: %v", err)
	}
	if st := kh.Wait(); st.Code != 0 || st.Signal != "" {
		t.Fatalf("SC-004: pod probe = %+v, want clean 0 (in-scope OK, out-of-scope denied)", st)
	}
	requireCleanCluster(t, b)
}

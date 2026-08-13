//go:build msb_e2e

// The M1.3 proof, run by `make test-msb` on a node with microsandbox
// installed (the default suite stays hermetic — research D6): the SAME
// declarations that run natively run inside microVMs, with the same op
// mapping on the topic (constitution III). Build-tagged, never skipped:
// without the tag these tests do not exist; with it they must pass.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/backend"
	"github.com/impire-io/soulstream-workloads/backend/msb"
	"github.com/impire-io/soulstream-workloads/backend/native"
	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/internal/natstest"
	"github.com/impire-io/soulstream-workloads/minter"
	"github.com/impire-io/soulstream-workloads/runner"
)

// requireNoSandboxes asserts SC-004's zero-leftovers clause: no soulstream-workloads-*
// sandbox survives its workload.
func requireNoSandboxes(t *testing.T) {
	t.Helper()
	out, err := exec.Command("msb", "ls").CombinedOutput()
	if err != nil {
		t.Fatalf("msb ls: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "soulstream-workloads-") {
		t.Fatalf("SC-004: leftover soulstream-workloads sandboxes:\n%s", out)
	}
}

// e2eMinter, buildCmdLinux, and buildInline moved to helpers_test.go (M2.1)
// — shared with the k8s_e2e suite.

// TestMsbLaunchAgentEndToEnd is SC-001 with the zero-diff clause enacted: ONE
// declaration value, marshalled and compared byte-for-byte across both runs;
// the node swaps the artifact build at the declared path between runs (host
// build for native, guest build for msb — node-side provisioning, per the
// spec's assumptions). The msb run must post the persona's turn and drive its
// work item to done exactly as the native control run does.
func TestMsbLaunchAgentEndToEnd(t *testing.T) {
	// The declared artifact path is stable; its content is provisioned per run.
	artifactPath := filepath.Join(t.TempDir(), "agent-echo")
	hostBuild := buildCmd(t, "github.com/impire-io/soulstream-workloads/cmd/agent-echo")
	linuxBuild := buildCmdLinux(t, "github.com/impire-io/soulstream-workloads/cmd/agent-echo")
	provision := func(src string) {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read %s: %v", src, err)
		}
		if err := os.WriteFile(artifactPath, data, 0o755); err != nil {
			t.Fatalf("provision artifact: %v", err)
		}
	}

	url, shutdown := natstest.StartJetStream(t)
	defer shutdown()
	ctx := context.Background()

	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	runnerClient, err := realm.NewClient(ctx, nc, realm.Config{Realm: "test-realm", Persona: "soulstream-workloads-runner"})
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

	run := func(be backend.Backend) {
		r := &runner.Runner{Minter: m, Backend: be, Realm: "test-realm", CredTTL: time.Hour, ScratchRoot: t.TempDir()}
		if err := r.Run(ctx, topic.Open(runnerClient, topicPath), d); err != nil {
			t.Fatalf("Run under %T: %v", be, err)
		}
	}

	// Native control run — M1.1's covered ground, here as the comparison arm.
	provision(hostBuild)
	run(native.New())

	// Same declaration value, byte-for-byte (US1 acceptance 2 / SC-001).
	declMsb, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(declNative, declMsb) {
		t.Fatalf("declaration changed between backends:\n%s\n%s", declNative, declMsb)
	}

	// The sandboxed run: node provisions the guest build at the same path.
	provision(linuxBuild)
	run(msb.New())

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
		if w.Status == topic.WorkDone && w.Author == "soulstream-workloads-runner" {
			done++
		}
	}
	if turns != 2 {
		t.Fatalf("SC-001: turns by researcher = %d, want 2 (native + msb); contributions=%+v", turns, mt.Contributions)
	}
	if done != 2 {
		t.Fatalf("SC-001: done work items = %d, want 2; workitems=%+v", done, mt.WorkItems)
	}
	requireNoSandboxes(t)
}

// TestMsbIsolationBoundary is SC-003: the probe that reads a host path
// SUCCEEDS under native and FAILS inside the sandbox — the wall is real.
func TestMsbIsolationBoundary(t *testing.T) {
	const probeSrc = `package main

import "os"

func main() {
	if _, err := os.ReadFile(os.Args[1]); err != nil {
		os.Exit(1)
	}
}
`
	secret := filepath.Join(t.TempDir(), "host-only.txt")
	if err := os.WriteFile(secret, []byte("host secret"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	m := e2eMinter(t, "nats://127.0.0.1:4222")
	cred, err := m.Mint(minter.Scope{Persona: "probe", Topic: "t-ab12"}, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	spec := func(artifact, scratch string) backend.LaunchSpec {
		return backend.LaunchSpec{
			Artifact:   artifact,
			Args:       []string{secret},
			Cred:       cred,
			Realm:      "test-realm",
			Topic:      "t-ab12",
			ScratchDir: scratch,
		}
	}

	hostProbe := buildInline(t, "probe", probeSrc, false)
	nh, err := native.New().Start(context.Background(), spec(hostProbe, filepath.Join(t.TempDir(), "n")))
	if err != nil {
		t.Fatalf("native start: %v", err)
	}
	if st := nh.Wait(); st.Code != 0 {
		t.Fatalf("native probe = %+v, want the host path readable", st)
	}

	linuxProbe := buildInline(t, "probe", probeSrc, true)
	mh, err := msb.New().Start(context.Background(), spec(linuxProbe, filepath.Join(t.TempDir(), "m")))
	if err != nil {
		t.Fatalf("msb start: %v", err)
	}
	if st := mh.Wait(); st.Code == 0 {
		t.Fatal("SC-003: the sandboxed probe read a host path — the isolation boundary is cosmetic")
	}
	requireNoSandboxes(t)
}

// TestMsbAgentCallsToolEndToEnd is SC-002: the M1.2 tool scenario with the
// tool INSIDE a microVM — discovery by name, uppercase round trip, stop →
// work.done, everything reaped.
func TestMsbAgentCallsToolEndToEnd(t *testing.T) {
	toolPath := buildCmdLinux(t, "github.com/impire-io/soulstream-workloads/cmd/tool-upper")

	url, shutdown := natstest.StartJetStream(t)
	defer shutdown()
	ctx := context.Background()

	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	runnerClient, err := realm.NewClient(ctx, nc, realm.Config{Realm: "test-realm", Persona: "soulstream-workloads-runner"})
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
	r := &runner.Runner{Minter: m, Backend: msb.New(), Realm: "test-realm", CredTTL: time.Hour, ScratchRoot: t.TempDir()}
	rw, err := r.Launch(ctx, topic.Open(runnerClient, topicPath), toolDecl)
	if err != nil {
		t.Fatalf("launch tool: %v", err)
	}
	// A test failure before the explicit Stop must not leak the sandbox.
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

	// Discovery window covers a microVM boot (research: cold-boot margin).
	subject := minter.ServiceSubject("uppercase")
	var reply *nats.Msg
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		reply, err = cnc.Request(subject, []byte("hi"), 500*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("SC-002: calling the sandboxed tool failed: %v", err)
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
		if w.Status == topic.WorkDone && w.Author == "soulstream-workloads-runner" {
			doneFound = true
		}
	}
	if !doneFound {
		t.Fatalf("SC-002: no completed tool work item; workitems=%+v", mt.WorkItems)
	}
	requireNoSandboxes(t)
}

// TestMsbCrashAbandons is SC-004: a workload dying nonzero inside its sandbox
// ends as work.abandon (the item reopens, claim cleared) and leaves nothing
// on the node.
func TestMsbCrashAbandons(t *testing.T) {
	crash := buildInline(t, "crash", "package main\n\nimport \"os\"\n\nfunc main() { os.Exit(3) }\n", true)

	url, shutdown := natstest.StartJetStream(t)
	defer shutdown()
	ctx := context.Background()

	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	runnerClient, err := realm.NewClient(ctx, nc, realm.Config{Realm: "test-realm", Persona: "soulstream-workloads-runner"})
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
	r := &runner.Runner{Minter: e2eMinter(t, url), Backend: msb.New(), Realm: "test-realm", CredTTL: time.Hour, ScratchRoot: t.TempDir()}
	if err := r.Run(ctx, topic.Open(runnerClient, topicPath), d); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mt, err := topic.Open(runnerClient, topicPath).Materialise(ctx)
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}
	var abandoned bool
	for _, w := range mt.WorkItems {
		if w.Author != "soulstream-workloads-runner" {
			continue
		}
		for _, ev := range w.Timeline {
			if ev.Kind == "abandon" && !ev.Void && ev.Author == "soulstream-workloads-runner" {
				abandoned = true
			}
		}
	}
	if !abandoned {
		t.Fatalf("SC-004: no abandon event for the crashed workload; workitems=%+v", mt.WorkItems)
	}
	requireNoSandboxes(t)
}

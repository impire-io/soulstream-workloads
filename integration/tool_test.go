package integration

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/backend/native"
	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/internal/natstest"
	"github.com/impire-io/soulstream-workloads/minter"
	"github.com/impire-io/soulstream-workloads/runner"
)

// TestAgentCallsToolEndToEnd is SC-001 + SC-002 for M1.2: soulstream-workloads launches a
// tool service; an agent discovers it by name and calls it (uppercase round
// trip); the tool's lifecycle is open+claim at launch and done at stop.
func TestAgentCallsToolEndToEnd(t *testing.T) {
	toolPath := buildCmd(t, "github.com/impire-io/soulstream-workloads/cmd/tool-upper")

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
		t.Fatalf("provision: %v", err)
	}
	h, err := topic.StartTopic(ctx, runnerClient, topic.StartTopicInput{Name: "tools"})
	if err != nil {
		t.Fatalf("start topic: %v", err)
	}
	topicPath := h.Path()

	acc, _ := nkeys.CreateAccount()
	accPub, _ := acc.PublicKey()
	seed, _ := acc.Seed()
	m, err := minter.NewSigningKeyMinter(seed, accPub, []string{url})
	if err != nil {
		t.Fatalf("minter: %v", err)
	}

	// Launch the tool (role=tool, a persistent service).
	toolDecl := declaration.Declaration{
		Role:      declaration.RoleTool,
		Lifecycle: declaration.LifecycleService,
		Persona:   "uppercase",
		Topic:     topicPath,
		Artifact:  "file://" + toolPath,
	}
	r := &runner.Runner{Minter: m, Backend: native.New(), Realm: "test-realm", CredTTL: time.Hour, ScratchRoot: t.TempDir()}
	rw, err := r.Launch(ctx, topic.Open(runnerClient, topicPath), toolDecl)
	if err != nil {
		t.Fatalf("launch tool: %v", err)
	}

	// The caller agent: a caller-scoped credential, discovering the tool by name.
	callerCred, err := m.Mint(minter.Scope{Role: declaration.RoleAgent, Persona: "researcher", Topic: topicPath}, time.Hour)
	if err != nil {
		t.Fatalf("mint caller: %v", err)
	}
	cnc, err := nats.Connect(url, nats.UserCredentials(writeCreds(t, callerCred)))
	if err != nil {
		t.Fatalf("caller connect: %v", err)
	}
	defer cnc.Close()

	// SC-001: discover by name (derive the subject) and call; retry while the
	// tool comes up (no responders yet).
	subject := minter.ServiceSubject("uppercase")
	var reply *nats.Msg
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		reply, err = cnc.Request(subject, []byte("hi"), 300*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("SC-001: calling the tool failed: %v", err)
	}
	if string(reply.Data) != "HI" {
		t.Fatalf("SC-001: reply = %q, want HI", reply.Data)
	}

	// Stop the tool → work.done.
	if err := rw.Stop(ctx); err != nil {
		t.Fatalf("stop tool: %v", err)
	}

	// SC-002: the tool's execution work item is done, driven by the runner.
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
		t.Fatalf("SC-002: no completed tool work item by the runner; workitems=%+v", mt.WorkItems)
	}
}

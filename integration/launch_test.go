// Package integration proves the M1.1 slice end to end: soulrealm launches a
// real agent process that participates in a real soulstream topic, and the
// workload's lifecycle shows up as work ops on that topic.
package integration

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"

	"github.com/impire-io/soulstream/realm"
	"github.com/impire-io/soulstream/topic"

	"github.com/impire-io/soulrealm/backend/native"
	"github.com/impire-io/soulrealm/declaration"
	"github.com/impire-io/soulrealm/internal/natstest"
	"github.com/impire-io/soulrealm/minter"
	"github.com/impire-io/soulrealm/runner"
)

func buildAgentEcho(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "agent-echo")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/impire-io/soulrealm/cmd/agent-echo")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build agent-echo: %v\n%s", err, out)
	}
	return bin
}

// TestLaunchAgentEndToEnd is SC-001 + SC-002: an agent launched by soulrealm
// posts a turn attributed to its persona, and the runner drives an execution
// work item to done on the same topic — all on the one control plane.
func TestLaunchAgentEndToEnd(t *testing.T) {
	agentPath := buildAgentEcho(t)

	url, shutdown := natstest.StartJetStream(t)
	defer shutdown()

	ctx := context.Background()

	// The runner connects as its own persona, provisions the realm, starts a topic.
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
		t.Fatalf("provision: %v", err)
	}
	h, err := topic.StartTopic(ctx, runnerClient, topic.StartTopicInput{Name: "planning"})
	if err != nil {
		t.Fatalf("start topic: %v", err)
	}
	topicPath := h.Path()

	// A throwaway account signs the minted creds; the mint + delivery path is
	// exercised end to end (the open in-process server does not enforce scope —
	// that is SC-003, which needs an operator-mode harness).
	acc, _ := nkeys.CreateAccount()
	accPub, _ := acc.PublicKey()
	seed, _ := acc.Seed()
	m, err := minter.NewSigningKeyMinter(seed, accPub, []string{url})
	if err != nil {
		t.Fatalf("minter: %v", err)
	}

	d := declaration.Declaration{
		Role:      declaration.RoleAgent,
		Lifecycle: declaration.LifecycleService,
		Persona:   "researcher",
		Topic:     topicPath,
		Artifact:  "file://" + agentPath,
	}

	r := &runner.Runner{
		Minter:      m,
		Backend:     native.New(),
		Realm:       "test-realm",
		CredTTL:     time.Hour,
		ScratchRoot: t.TempDir(),
	}
	if err := r.Run(ctx, topic.Open(runnerClient, topicPath), d); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mt, err := topic.Open(runnerClient, topicPath).Materialise(ctx)
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}

	// SC-001: a turn authored by the agent's persona.
	var turnFound bool
	for _, c := range mt.Contributions {
		if c.Author == "researcher" && strings.Contains(c.Body, "researcher") {
			turnFound = true
		}
	}
	if !turnFound {
		t.Fatalf("SC-001: no turn attributed to researcher; contributions=%+v", mt.Contributions)
	}

	// SC-002: an execution work item driven to done by the runner persona.
	var doneFound bool
	for _, w := range mt.WorkItems {
		if w.Status == topic.WorkDone && w.Author == "soulrealm-runner" {
			doneFound = true
		}
	}
	if !doneFound {
		t.Fatalf("SC-002: no completed work item by the runner; workitems=%+v", mt.WorkItems)
	}
}

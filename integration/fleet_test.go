// M3.1's gate (hq design 0003-fleet.md §8): two real node processes'
// worth of fleet behaviour against one realm — contested placement
// decided by the log alone, a dead node's work reclaimed within bound,
// a live owner never reclaimed, and the probe traffic nowhere on the
// stream.
package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/backend/native"
	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/fleet"
	"github.com/impire-io/soulstream-workloads/internal/natstest"
	"github.com/impire-io/soulstream-workloads/minter"
	"github.com/impire-io/soulstream-workloads/runner"
)

type fleetRig struct {
	url       string
	topicPath string
	submitter *topic.Handle
	nodes     map[string]*fleet.Node
	handles   map[string]*topic.Handle
	agentPath string
}

func newFleetRig(t *testing.T, nodeIDs ...string) *fleetRig {
	t.Helper()
	ctx := context.Background()
	agentPath := buildCmd(t, "github.com/impire-io/soulstream-workloads/cmd/agent-echo")
	url, shutdown := natstest.StartJetStream(t)
	t.Cleanup(shutdown)

	connect := func(persona string) (*nats.Conn, *realm.Client) {
		nc, err := nats.Connect(url)
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		t.Cleanup(nc.Close)
		c, err := realm.NewClient(ctx, nc, realm.Config{Realm: "test-realm", Persona: persona})
		if err != nil {
			t.Fatalf("client %s: %v", persona, err)
		}
		return nc, c
	}

	_, submitterClient := connect("submitter")
	if _, err := submitterClient.Provision(ctx); err != nil {
		t.Fatalf("provision: %v", err)
	}
	h, err := topic.StartTopic(ctx, submitterClient, topic.StartTopicInput{Name: "fleet"})
	if err != nil {
		t.Fatalf("start topic: %v", err)
	}

	acc, _ := nkeys.CreateAccount()
	accPub, _ := acc.PublicKey()
	seed, _ := acc.Seed()

	rig := &fleetRig{
		url: url, topicPath: h.Path(), submitter: h, agentPath: agentPath,
		nodes: map[string]*fleet.Node{}, handles: map[string]*topic.Handle{},
	}
	for _, id := range nodeIDs {
		nc, c := connect(id)
		m, err := minter.NewSigningKeyMinter(seed, accPub, []string{url})
		if err != nil {
			t.Fatalf("minter: %v", err)
		}
		nh := topic.Open(c, h.Path())
		if _, err := nh.Materialise(ctx); err != nil {
			t.Fatalf("materialise %s: %v", id, err)
		}
		n := &fleet.Node{
			ID: id, Conn: nc,
			Runner: &runner.Runner{
				Minter: m, Backend: native.New(), Realm: "test-realm",
				CredTTL: time.Hour, ScratchRoot: t.TempDir(),
			},
			Reclaim: 500 * time.Millisecond, ProbeTimeout: 200 * time.Millisecond,
		}
		if err := n.Start(); err != nil {
			t.Fatalf("node %s: %v", id, err)
		}
		t.Cleanup(n.Stop)
		rig.nodes[id] = n
		rig.handles[id] = nh
	}
	return rig
}

func (r *fleetRig) decl(persona string) declaration.Declaration {
	return declaration.Declaration{
		Role:      declaration.RoleAgent,
		Lifecycle: declaration.LifecycleService,
		Persona:   persona,
		Topic:     r.topicPath,
		Artifact:  "file://" + r.agentPath,
	}
}

// TestFleetContestedPlacement is §8.1: two nodes racing every
// submission, each item run by exactly one node, the outcome
// reconstructible from the log alone — and §8.3's other half, that the
// probe traffic never lands on the stream.
func TestFleetContestedPlacement(t *testing.T) {
	ctx := context.Background()
	rig := newFleetRig(t, "node-a", "node-b")

	const placements = 4
	var ids []string
	for i := 0; i < placements; i++ {
		id, err := fleet.Submit(ctx, rig.submitter, rig.decl("researcher"))
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		ids = append(ids, id)
	}

	// Both nodes race every item, back to back. The log decides.
	won := map[string]string{}
	for _, id := range ids {
		var winners []string
		for _, nodeID := range []string{"node-a", "node-b"} {
			p, err := rig.nodes[nodeID].TryPlace(ctx, rig.handles[nodeID], id)
			switch {
			case err == nil:
				winners = append(winners, nodeID)
				t.Cleanup(func() { _ = rig.nodes[nodeID].Release(context.Background(), rig.handles[nodeID], p) })
			case strings.Contains(err.Error(), "another node won"):
			default:
				t.Fatalf("%s placing %s: %v", nodeID, id, err)
			}
		}
		if len(winners) != 1 {
			t.Fatalf("placement %s ran on %v — want exactly one node", id, winners)
		}
		won[id] = winners[0]
	}

	// Replay alone reconstructs placement: every item claimed, exactly
	// one non-void claim each, the owner the node that launched.
	mt, err := rig.submitter.Materialise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, item := range mt.WorkItems {
		owner, isPlacement := won[item.ID]
		if !isPlacement {
			continue
		}
		seen++
		if item.Owner != owner {
			t.Errorf("item %s: log says %q ran it, the fleet says %q", item.ID, item.Owner, owner)
		}
		live := 0
		for _, ev := range item.Timeline {
			if ev.Kind == "claim" && !ev.Void {
				live++
			}
		}
		if live != 1 {
			t.Errorf("item %s: %d live claims, want exactly 1", item.ID, live)
		}
	}
	if seen != placements {
		t.Fatalf("replay found %d placements, want %d", seen, placements)
	}

	// §8.3: the probe subject carries traffic, and the stream never sees
	// it — the op-log holds records only.
	for _, item := range mt.WorkItems {
		if strings.Contains(item.Title, "FLEET") || strings.Contains(item.Body, "SOULSTREAM.SVC") {
			t.Fatalf("probe traffic leaked onto the stream: %+v", item)
		}
	}
}

// TestFleetReclaimsADeadNode is §8.2 and §8.3: a live owner is never
// reclaimed; a silent one is, within the bound, as an ordinary
// work.abandon — and the item then runs on the surviving node with a
// fresh claim on the record.
func TestFleetReclaimsADeadNode(t *testing.T) {
	ctx := context.Background()
	rig := newFleetRig(t, "node-a", "node-b")
	a, b := rig.nodes["node-a"], rig.nodes["node-b"]
	ha, hb := rig.handles["node-a"], rig.handles["node-b"]

	id, err := fleet.Submit(ctx, rig.submitter, rig.decl("researcher"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.TryPlace(ctx, ha, id)
	if err != nil {
		t.Fatalf("node-a place: %v", err)
	}

	// §8.3: past the bound but answering probes — never reclaimed.
	time.Sleep(600 * time.Millisecond)
	reopened, err := b.Sweep(ctx, hb)
	if err != nil {
		t.Fatalf("sweep with a live owner: %v", err)
	}
	if len(reopened) != 0 {
		t.Fatalf("a live owner was reclaimed: %v", reopened)
	}

	// The node dies: its probe answers stop. The workload keeps running
	// (an orphan), exactly as a SIGKILLed node's would.
	a.Stop()

	// §8.2: the surviving node reclaims within the bound.
	deadline := time.Now().Add(10 * time.Second)
	for {
		reopened, err = b.Sweep(ctx, hb)
		if err != nil {
			t.Fatalf("sweep: %v", err)
		}
		if len(reopened) == 1 && reopened[0] == id {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the dead node's placement was never reclaimed")
		}
		time.Sleep(100 * time.Millisecond)
	}

	// The reclaim is an ordinary abandon: the item is open again, and the
	// survivor wins the fresh race on the record.
	p2, err := b.TryPlace(ctx, hb, id)
	if err != nil {
		t.Fatalf("node-b re-place: %v", err)
	}
	defer func() { _ = b.Release(context.Background(), hb, p2) }()

	mt, err := rig.submitter.Materialise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var item topic.WorkItem
	for _, w := range mt.WorkItems {
		if w.ID == id {
			item = w
		}
	}
	if item.Owner != "node-b" || item.Status != topic.WorkClaimed {
		t.Fatalf("after reclaim: owner=%q status=%q", item.Owner, item.Status)
	}
	kinds := make([]string, 0, len(item.Timeline))
	for _, ev := range item.Timeline {
		if !ev.Void {
			kinds = append(kinds, ev.Kind)
		}
	}
	if strings.Join(kinds, ",") != "claim,abandon,claim" {
		t.Fatalf("reclaim timeline = %v, want claim,abandon,claim", kinds)
	}
	// No double-close: the abandoned run was never completed by anyone.
	for _, ev := range item.Timeline {
		if ev.Kind == "done" {
			t.Fatal("a reclaimed placement was also closed done — double close")
		}
	}
	_ = p // the orphaned run is the dead node's; the rig tears it down
}

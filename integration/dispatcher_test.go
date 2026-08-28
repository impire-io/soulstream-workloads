// The specs/011 gate (hq design 0007 §8, bars 1-3): a standing
// dispatcher makes submit-and-forget real. The submitter walks away and
// the mention is still answered exactly once; a restart resumes from the
// log alone; a crash posts nothing and the successor answers once; two
// nodes settle every contested placement on the record and the survivor
// takes over a dead peer's work; and the 0006 budget still speaks,
// because the dispatcher serves through the same engine and adds no
// admission point of its own.
//
// Bars 4-5 (the provider secret and the session-admission loop) belong to
// the inference work design 0007 §3 holds; nothing here builds for it.
package integration

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/dispatcher"
	"github.com/impire-io/soulstream-workloads/fleet"
	"github.com/impire-io/soulstream-workloads/internal/natstest"
	"github.com/impire-io/soulstream-workloads/wrap"
)

// dispatcherRig is a hermetic realm for the specs/011 scenarios: a person
// who submits and posts, a placement topic the nodes race, and a room the
// served agents work in. The two topics are deliberately separate — an
// agent's own turns are not placement activity.
type dispatcherRig struct {
	url        string
	owner      *realm.Client
	placements string
	room       string
}

func startDispatcherRealm(t *testing.T) *dispatcherRig {
	t.Helper()
	url, shutdown := natstest.StartJetStream(t)
	t.Cleanup(shutdown)
	rig := &dispatcherRig{url: url}
	rig.owner = rig.connect(t, "owner")
	if _, err := rig.owner.Provision(context.Background()); err != nil {
		t.Fatalf("provision: %v", err)
	}
	rig.placements = rig.startTopic(t, "placements")
	rig.room = rig.startTopic(t, "room")
	return rig
}

func (r *dispatcherRig) connect(t *testing.T, persona string) *realm.Client {
	t.Helper()
	c, err := r.dial(persona)
	if err != nil {
		t.Fatalf("connect %s: %v", persona, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// dial is the non-fatal half: the dispatcher's ConnectAgent hook runs on
// its own goroutine, where t.Fatal is not allowed.
func (r *dispatcherRig) dial(persona string) (*realm.Client, error) {
	nc, err := nats.Connect(r.url)
	if err != nil {
		return nil, err
	}
	c, err := realm.NewClient(context.Background(), nc, realm.Config{Realm: "test-realm", Persona: persona})
	if err != nil {
		nc.Close()
		return nil, err
	}
	return c, nil
}

func (r *dispatcherRig) startTopic(t *testing.T, name string) string {
	t.Helper()
	h, err := topic.StartTopic(context.Background(), r.owner, topic.StartTopicInput{Name: name})
	if err != nil {
		t.Fatalf("start topic %s: %v", name, err)
	}
	return h.Path()
}

// submit puts one declaration on the placement topic as a persona who
// then goes away — submit-and-forget's first half.
func (r *dispatcherRig) submit(t *testing.T, as string, d declaration.Declaration) string {
	t.Helper()
	c := r.connect(t, as)
	id, err := fleet.Submit(context.Background(), topic.Open(c, r.placements), d)
	if err != nil {
		t.Fatalf("submit %s: %v", d.Persona, err)
	}
	return id
}

// dispatchNode is one dispatcher under test, with the two ends of design
// 0007 §6 available separately: stop drains, and dropConnections plus
// stop is the crash.
type dispatchNode struct {
	id     string
	client *realm.Client
	cancel context.CancelFunc
	done   chan struct{}

	mu     sync.Mutex
	agents []*realm.Client
}

func (r *dispatcherRig) startNode(t *testing.T, id string, invoke wrap.Invoker, log *slog.Logger) *dispatchNode {
	t.Helper()
	n := &dispatchNode{id: id, client: r.connect(t, id), done: make(chan struct{})}
	d := &dispatcher.Dispatcher{
		Node:       id,
		Client:     n.client,
		Placements: r.placements,
		ConnectAgent: func(_ context.Context, persona string) (*realm.Client, error) {
			c, err := r.dial(persona)
			if err != nil {
				return nil, err
			}
			n.mu.Lock()
			n.agents = append(n.agents, c)
			n.mu.Unlock()
			return c, nil
		},
		Engine: wrap.Config{Template: dispatcherTemplate(), Scratch: t.TempDir(),
			RunTimeout: 5 * time.Second, Retries: 1},
		Invoke:       invoke,
		Reclaim:      time.Second,
		ProbeTimeout: 200 * time.Millisecond,
		SweepEvery:   250 * time.Millisecond,
		PollEvery:    150 * time.Millisecond,
		RaceBackoff:  500 * time.Millisecond,
		DrainTimeout: 15 * time.Second,
		Log:          log,
	}
	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel
	go func() {
		defer close(n.done)
		if err := d.Run(ctx); err != nil {
			log.Error("dispatcher_run_ended", "node", id, "err", err)
		}
	}()
	t.Cleanup(func() { n.stop(t) })
	return n
}

// stop is the drain end: the context ends, the engines are cancelled and
// waited for. Idempotent, so the cleanup after an explicit stop is free.
func (n *dispatchNode) stop(t *testing.T) {
	t.Helper()
	n.cancel()
	select {
	case <-n.done:
	case <-time.After(30 * time.Second):
		t.Errorf("dispatcher %s did not stop", n.id)
	}
}

// dropConnections is the crash end: every connection this node holds —
// its own and its engines' — dies where it stands, exactly as a SIGKILL
// leaves them. Nothing in flight can post after this.
func (n *dispatchNode) dropConnections() {
	n.mu.Lock()
	for _, c := range n.agents {
		c.Conn().Close()
	}
	n.mu.Unlock()
	n.client.Conn().Close()
}

// dispatcherTemplate carries the persona into the prompt, so one invoker
// can script every agent a node serves — the dispatcher has one harness
// seam for all of them.
func dispatcherTemplate() wrap.Template {
	return wrap.Template{
		Command: []string{"/usr/bin/true"},
		Prompt:  "PERSONA={{PERSONA}}\nAUTHOR={{AUTHOR}}\nBODY={{BODY}}",
		Terminal: wrap.TerminalMap{
			TypeField: "type", TerminalValue: "result", TextField: "result",
		},
	}
}

func promptPersona(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		if v, ok := strings.CutPrefix(line, "PERSONA="); ok {
			return v
		}
	}
	return ""
}

func scriptedHarness(script func(persona, author, body string) (string, bool)) wrap.Invoker {
	return func(_ context.Context, spec wrap.RunSpec) wrap.HarnessResult {
		author, body := promptFields(spec.Prompt)
		text, ok := script(promptPersona(spec.Prompt), author, body)
		if !ok {
			return wrap.HarnessResult{OK: false, Detail: "script refused"}
		}
		return wrap.HarnessResult{OK: true, Text: text, Detail: "scripted"}
	}
}

// mentionAgent is the declaration a dispatcher serves. The artifact is
// required by the schema and unused on this path: the engine runs the
// node's harness template, never the declared executable.
func mentionAgent(persona, room string) declaration.Declaration {
	return declaration.Declaration{
		Role:      declaration.RoleAgent,
		Lifecycle: declaration.LifecycleService,
		Persona:   persona,
		Topic:     room,
		Artifact:  "file:///bin/true",
		Wake:      []declaration.WakeEntry{{Kind: declaration.WakeMention}},
	}
}

func placementItem(t *testing.T, c *realm.Client, path, itemID string) topic.WorkItem {
	t.Helper()
	view, err := topic.Open(c, path).Materialise(context.Background())
	if err != nil {
		t.Fatalf("materialise placements: %v", err)
	}
	for _, item := range view.WorkItems {
		if item.ID == itemID {
			return item
		}
	}
	t.Fatalf("no placement %s on %s", itemID, path)
	return topic.WorkItem{}
}

// liveEvents is the item's timeline with void events dropped — the
// history a lost claim leaves behind is record, not a second owner.
func liveEvents(item topic.WorkItem) []string {
	var kinds []string
	for _, ev := range item.Timeline {
		if !ev.Void {
			kinds = append(kinds, ev.Kind)
		}
	}
	return kinds
}

func waitFor(t *testing.T, what string, limit time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// settleTurns waits until the authors' turn count in a topic has been
// stable for quiet, and reports the final count and whether it settled.
func settleTurns(t *testing.T, c *realm.Client, path string, quiet, limit time.Duration, authors ...string) (int, bool) {
	t.Helper()
	count := func() int {
		n := 0
		for _, a := range authors {
			n += len(turnsBy(t, c, path, a))
		}
		return n
	}
	deadline := time.Now().Add(limit)
	last, lastChange := -1, time.Now()
	for time.Now().Before(deadline) {
		if n := count(); n != last {
			last, lastChange = n, time.Now()
		} else if time.Since(lastChange) >= quiet {
			return n, true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return count(), false
}

// TestDispatcherSubmitAndForget is design 0007 §8.1's first two halves:
// the submitter is gone before the agent ever runs and the mention is
// still answered exactly once; a dispatcher restart resumes from the log
// with no new op and answers nothing twice.
func TestDispatcherSubmitAndForget(t *testing.T) {
	rig := startDispatcherRealm(t)
	id := rig.submit(t, "submitter", mentionAgent("clerk", rig.room))

	script := scriptedHarness(func(persona, _, _ string) (string, bool) {
		return "answered by " + persona, true
	})
	node := rig.startNode(t, "node-a", script, slog.New(&refusalCounter{}))

	waitFor(t, "the placement to be claimed", 20*time.Second, func() bool {
		item := placementItem(t, rig.owner, rig.placements, id)
		return item.Status == topic.WorkClaimed && item.Owner == "node-a"
	})

	post(t, rig.owner, rig.room, "@clerk are you there?")
	waitTurns(t, rig.owner, rig.room, "clerk", 1)

	before := placementItem(t, rig.owner, rig.placements, id)
	if got := liveEvents(before); len(got) != 1 || got[0] != "claim" {
		t.Fatalf("placement timeline = %v, want a single claim", got)
	}

	// The restart. The record alone says what this node owns: no new op,
	// no handshake, and the answered mention stays answered.
	node.stop(t)
	rig.startNode(t, "node-a", script, slog.New(&refusalCounter{}))

	post(t, rig.owner, rig.room, "@clerk still there?")
	waitTurns(t, rig.owner, rig.room, "clerk", 2)

	after := placementItem(t, rig.owner, rig.placements, id)
	if len(after.Timeline) != len(before.Timeline) {
		t.Fatalf("resume published %d new event(s) — the record is the position, not a handshake",
			len(after.Timeline)-len(before.Timeline))
	}
	if after.Owner != "node-a" || after.Status != topic.WorkClaimed {
		t.Fatalf("after resume: owner=%q status=%q", after.Owner, after.Status)
	}
}

// TestDispatcherCrashPostsNothingAndResumesOnce is design 0007 §8.1's
// third half and §6's crash arm: connections dropped mid-run leave the
// topic untouched, and the successor serves that same wake exactly once
// on the deterministic outcome id.
func TestDispatcherCrashPostsNothingAndResumesOnce(t *testing.T) {
	rig := startDispatcherRealm(t)
	id := rig.submit(t, "submitter", mentionAgent("clerk", rig.room))

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var runs atomic.Int64
	invoke := func(_ context.Context, _ wrap.RunSpec) wrap.HarnessResult {
		if runs.Add(1) == 1 {
			started <- struct{}{}
			<-release
		}
		return wrap.HarnessResult{OK: true, Text: "answered", Detail: "scripted"}
	}
	node := rig.startNode(t, "node-a", invoke, slog.New(&refusalCounter{}))

	waitFor(t, "the placement to be claimed", 20*time.Second, func() bool {
		item := placementItem(t, rig.owner, rig.placements, id)
		return item.Status == topic.WorkClaimed && item.Owner == "node-a"
	})
	post(t, rig.owner, rig.room, "@clerk are you there?")

	select {
	case <-started:
	case <-time.After(20 * time.Second):
		t.Fatal("the harness never ran")
	}

	// The kill: every connection dies before anything is discharged, so
	// the finished harness has nowhere to post its answer.
	node.dropConnections()
	close(release)
	time.Sleep(time.Second) // let the discharge fail against dead connections
	node.stop(t)

	if got := turnsBy(t, rig.owner, rig.room, "clerk"); len(got) != 0 {
		t.Fatalf("a crashed dispatcher posted %d outcome(s), want 0", len(got))
	}

	rig.startNode(t, "node-a", invoke, slog.New(&refusalCounter{}))
	waitTurns(t, rig.owner, rig.room, "clerk", 1)
	if got := runs.Load(); got < 2 {
		t.Fatalf("harness runs = %d, want the crashed attempt and the successor's", got)
	}
}

// TestDispatcherTwoNodesRaceAndFailOver is design 0007 §8.2: contested
// placements settle on the record with one owner and one live claim
// each; a dead node's work reclaims as an ordinary abandon and the
// survivor answers a wake posted in the failover window exactly once;
// none of the probe traffic reaches the stream.
func TestDispatcherTwoNodesRaceAndFailOver(t *testing.T) {
	rig := startDispatcherRealm(t)
	personas := []string{"clerk-a", "clerk-b", "clerk-c"}
	ids := map[string]string{}
	for _, p := range personas {
		ids[p] = rig.submit(t, "submitter", mentionAgent(p, rig.room))
	}

	script := scriptedHarness(func(persona, _, _ string) (string, bool) {
		return "answered by " + persona, true
	})
	nodes := map[string]*dispatchNode{
		"node-a": rig.startNode(t, "node-a", script, slog.New(&refusalCounter{})),
		"node-b": rig.startNode(t, "node-b", script, slog.New(&refusalCounter{})),
	}

	waitFor(t, "every placement to be claimed", 30*time.Second, func() bool {
		for _, id := range ids {
			if placementItem(t, rig.owner, rig.placements, id).Status != topic.WorkClaimed {
				return false
			}
		}
		return true
	})

	for p, id := range ids {
		item := placementItem(t, rig.owner, rig.placements, id)
		if got := liveEvents(item); len(got) != 1 || got[0] != "claim" {
			t.Errorf("placement %s: timeline %v, want exactly one live claim", p, got)
		}
		if _, known := nodes[item.Owner]; !known {
			t.Errorf("placement %s: owner %q is not a node in this fleet", p, item.Owner)
		}
	}

	// Kill whichever node the log gave clerk-a, so the failover is real
	// rather than an accident of the race.
	victimID := placementItem(t, rig.owner, rig.placements, ids["clerk-a"]).Owner
	survivorID := "node-a"
	if victimID == "node-a" {
		survivorID = "node-b"
	}
	nodes[victimID].dropConnections()
	nodes[victimID].stop(t)

	// A wake inside the failover window: nobody is serving clerk-a yet.
	post(t, rig.owner, rig.room, "@clerk-a are you there?")

	waitFor(t, "the survivor to reclaim clerk-a's placement", 30*time.Second, func() bool {
		item := placementItem(t, rig.owner, rig.placements, ids["clerk-a"])
		return item.Status == topic.WorkClaimed && item.Owner == survivorID
	})
	reclaimed := placementItem(t, rig.owner, rig.placements, ids["clerk-a"])
	if got := strings.Join(liveEvents(reclaimed), ","); got != "claim,abandon,claim" {
		t.Fatalf("reclaim timeline = %v, want claim,abandon,claim", got)
	}
	for _, ev := range reclaimed.Timeline {
		if ev.Kind == "done" {
			t.Fatal("a reclaimed placement was also closed done — double close")
		}
	}

	waitTurns(t, rig.owner, rig.room, "clerk-a", 1)

	// The probe traffic is evidence, never record (design 0003 §1).
	view, err := topic.Open(rig.owner, rig.placements).Materialise(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range view.WorkItems {
		if strings.Contains(item.Title, "SOULSTREAM.SVC") || strings.Contains(item.Body, "SOULSTREAM.SVC") {
			t.Fatalf("probe traffic leaked onto the stream: %+v", item)
		}
	}
	for _, c := range view.Contributions {
		if strings.Contains(c.Body, "SOULSTREAM.SVC") {
			t.Fatalf("probe traffic leaked onto the stream: %+v", c)
		}
	}
}

// TestDispatcherBudgetHaltsUncooperativeCycle is design 0007 §8.3's first
// half: the declared budget still halts a runaway through the dispatcher
// path, op-lessly and loudly. The dispatcher adds no admission point —
// this measures that the engine's seam stays reachable.
func TestDispatcherBudgetHaltsUncooperativeCycle(t *testing.T) {
	rig := startDispatcherRealm(t)
	const maxHops = 4
	budget := func(d declaration.Declaration) declaration.Declaration {
		d.Budget = &declaration.BudgetSpec{
			MaxHops: maxHops,
			Window:  &declaration.WindowSpec{Max: 100, Per: "1h"},
		}
		return d
	}
	idA := rig.submit(t, "submitter", budget(mentionAgent("agent-a", rig.room)))
	idB := rig.submit(t, "submitter", budget(mentionAgent("agent-b", rig.room)))

	counter := &refusalCounter{}
	rig.startNode(t, "node-a", scriptedHarness(func(persona, _, _ string) (string, bool) {
		if persona == "agent-a" {
			return "ping @agent-b", true
		}
		return "pong @agent-a", true
	}), slog.New(counter))

	waitFor(t, "both agents to be served", 30*time.Second, func() bool {
		for _, id := range []string{idA, idB} {
			if placementItem(t, rig.owner, rig.placements, id).Status != topic.WorkClaimed {
				return false
			}
		}
		return true
	})

	post(t, rig.owner, rig.room, "@agent-a start")
	n, settled := settleTurns(t, rig.owner, rig.room, 2*time.Second, 40*time.Second, "agent-a", "agent-b")
	if !settled {
		t.Fatalf("the cascade did not settle under the declared budget (%d agent turns)", n)
	}
	if n != maxHops {
		t.Fatalf("agent turns = %d, want exactly the declared max_hops = %d", n, maxHops)
	}
	if counter.n.Load() < 1 {
		t.Fatal("expected at least one loud refusal on the dispatcher path")
	}
	for _, a := range []string{"agent-a", "agent-b"} {
		for _, c := range turnsBy(t, rig.owner, rig.room, a) {
			if strings.Contains(c.Body, "could not answer") {
				t.Fatalf("a refusal posted testimony: %q", c.Body)
			}
		}
	}
}

// TestDispatcherDelegationSurvivesDefaults is design 0007 §8.3's second
// half: a legitimate owner→A→B→A delegation completes through the
// dispatcher with zero refusals under the engine's default budget.
func TestDispatcherDelegationSurvivesDefaults(t *testing.T) {
	rig := startDispatcherRealm(t)
	idA := rig.submit(t, "submitter", mentionAgent("agent-a", rig.room))
	idB := rig.submit(t, "submitter", mentionAgent("agent-b", rig.room))

	counter := &refusalCounter{}
	rig.startNode(t, "node-a", scriptedHarness(func(persona, _, body string) (string, bool) {
		switch {
		case persona == "agent-a" && strings.Contains(body, "please summarise"):
			return "on it — @agent-b fetch the data", true
		case persona == "agent-a" && strings.Contains(body, "data:"):
			return "done: sunny, 24C", true
		case persona == "agent-b" && strings.Contains(body, "fetch the data"):
			return "@agent-a data: sunny, 24C", true
		}
		return "", false
	}), slog.New(counter))

	waitFor(t, "both agents to be served", 30*time.Second, func() bool {
		for _, id := range []string{idA, idB} {
			if placementItem(t, rig.owner, rig.placements, id).Status != topic.WorkClaimed {
				return false
			}
		}
		return true
	})

	post(t, rig.owner, rig.room, "@agent-a please summarise the weather")
	n, settled := settleTurns(t, rig.owner, rig.room, 2*time.Second, 40*time.Second, "agent-a", "agent-b")
	if !settled || n != 3 {
		t.Fatalf("delegation: settled=%v agent turns=%d, want settled with exactly 3", settled, n)
	}
	if got := counter.n.Load(); got != 0 {
		t.Fatalf("refusals = %d inside a legitimate chain, want 0", got)
	}
}

// TestDispatcherLeavesTheRunnerPathAlone: a placement the wake engine
// cannot serve is not claimed, not served, and not abandoned — it stays
// open for the fleet's Runner path, so a dispatcher node and a runner
// node can share one realm and one placement topic.
func TestDispatcherLeavesTheRunnerPathAlone(t *testing.T) {
	rig := startDispatcherRealm(t)

	tool := declaration.Declaration{
		Role: declaration.RoleTool, Lifecycle: declaration.LifecycleService,
		Persona: "fetcher", Topic: rig.room, Artifact: "file:///bin/true",
	}
	quiet := mentionAgent("quiet", rig.room)
	quiet.Wake = nil

	toolID := rig.submit(t, "submitter", tool)
	quietID := rig.submit(t, "submitter", quiet)
	servedID := rig.submit(t, "submitter", mentionAgent("clerk", rig.room))

	rig.startNode(t, "node-a", scriptedHarness(func(persona, _, _ string) (string, bool) {
		return "answered by " + persona, true
	}), slog.New(&refusalCounter{}))

	waitFor(t, "the servable placement to be claimed", 20*time.Second, func() bool {
		return placementItem(t, rig.owner, rig.placements, servedID).Status == topic.WorkClaimed
	})
	time.Sleep(time.Second) // several more scans over the same open items

	for name, id := range map[string]string{"tool": toolID, "wakeless agent": quietID} {
		item := placementItem(t, rig.owner, rig.placements, id)
		if item.Status != topic.WorkOpen {
			t.Errorf("the %s placement is %q — the dispatcher took work it cannot run", name, item.Status)
		}
		if got := liveEvents(item); len(got) != 0 {
			t.Errorf("the %s placement gained events %v — it should be untouched", name, got)
		}
	}
}

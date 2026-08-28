package dispatcher

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/realm"

	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/internal/natstest"
	"github.com/impire-io/soulstream-workloads/wrap"
)

func testTemplate() wrap.Template {
	return wrap.Template{
		Command:  []string{"/usr/bin/true"},
		Prompt:   "{{BODY}}",
		Terminal: wrap.TerminalMap{TypeField: "type", TerminalValue: "result", TextField: "result"},
	}
}

// nodeClient is one connected node persona — the refusal table needs a
// real client because the node name and the claim attribution come from
// the connection, not from configuration.
func nodeClient(t *testing.T, persona string) *realm.Client {
	t.Helper()
	url, shutdown := natstest.StartJetStream(t)
	t.Cleanup(shutdown)
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	c, err := realm.NewClient(context.Background(), nc, realm.Config{Realm: "test-realm", Persona: persona})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// Only an agent with something that wakes it is engine-servable;
// everything else is the fleet Runner path's and the dispatcher must
// leave it alone (FR-005).
func TestServable(t *testing.T) {
	agent := declaration.Declaration{Role: declaration.RoleAgent, Persona: "clerk"}
	waking := agent
	waking.Wake = []declaration.WakeEntry{{Kind: declaration.WakeMention}}
	tool := declaration.Declaration{Role: declaration.RoleTool, Persona: "fetcher"}

	for _, tc := range []struct {
		name string
		decl declaration.Declaration
		want bool
	}{
		{"agent with a wake set", waking, true},
		{"agent with no wake set", agent, false},
		{"tool", tool, false},
	} {
		if got := Servable(tc.decl); got != tc.want {
			t.Errorf("Servable(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A configuration that could only fail later, one placement at a time,
// fails at Run instead (FR-007, FR-012).
func TestRunRefusesIncompleteConfiguration(t *testing.T) {
	client := nodeClient(t, "node-a")
	connect := func(context.Context, string) (*realm.Client, error) { return nil, nil }

	base := func() *Dispatcher {
		return &Dispatcher{
			Client: client, Placements: "t-ab12", ConnectAgent: connect,
			Engine: wrap.Config{Template: testTemplate(), Scratch: t.TempDir()},
		}
	}
	for _, tc := range []struct {
		name string
		mod  func(*Dispatcher)
		want string
	}{
		{"no client", func(d *Dispatcher) { d.Client = nil }, "node client is required"},
		{"no placement topic", func(d *Dispatcher) { d.Placements = "" }, "placement topic path is required"},
		{"no connect hook", func(d *Dispatcher) { d.ConnectAgent = nil }, "ConnectAgent hook is required"},
		{"engine fixes a persona", func(d *Dispatcher) { d.Engine.Persona = "clerk" }, "must not fix a persona"},
		{"no scratch root", func(d *Dispatcher) { d.Engine.Scratch = "" }, "scratch root is required"},
		{"no template", func(d *Dispatcher) { d.Engine.Template = wrap.Template{} }, "template.command is required"},
		{"node is not the connected persona", func(d *Dispatcher) { d.Node = "node-b" }, "authorship is mechanical"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := base()
			tc.mod(d)
			err := d.Run(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run() = %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

// The node name defaults to the connection's persona: claims are
// attributed mechanically, so the two can never legitimately differ.
func TestNodeNameDefaultsToTheConnectedPersona(t *testing.T) {
	d := &Dispatcher{
		Client: nodeClient(t, "node-a"), Placements: "t-ab12",
		ConnectAgent: func(context.Context, string) (*realm.Client, error) { return nil, nil },
		Engine:       wrap.Config{Template: testTemplate(), Scratch: t.TempDir()},
	}
	if err := d.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if d.node != "node-a" {
		t.Fatalf("node = %q, want the connected persona", d.node)
	}
}

// The sweep cadence follows the reclaim bound rather than needing a
// second knob kept in step with the first.
func TestCadenceDefaults(t *testing.T) {
	d := &Dispatcher{}
	if got := d.sweepEvery(); got != defaultReclaim/4 {
		t.Errorf("sweepEvery = %s, want a quarter of the default reclaim bound", got)
	}
	d.Reclaim = time.Second
	if got := d.sweepEvery(); got != 250*time.Millisecond {
		t.Errorf("sweepEvery = %s, want a quarter of a 1s reclaim bound", got)
	}
	d.Reclaim = 100 * time.Millisecond
	if got := d.sweepEvery(); got != minSweepEvery {
		t.Errorf("sweepEvery = %s, want the floor for a very short bound", got)
	}
	d.SweepEvery = 3 * time.Second
	if got := d.sweepEvery(); got != 3*time.Second {
		t.Errorf("sweepEvery = %s, want the configured value", got)
	}
	if got := d.pollEvery(); got != defaultPollEvery {
		t.Errorf("pollEvery = %s, want the default", got)
	}
	if got := d.raceBackoff(); got != defaultRaceBackoff {
		t.Errorf("raceBackoff = %s, want the default", got)
	}
	if got := d.drainTimeout(); got != defaultDrainTimeout {
		t.Errorf("drainTimeout = %s, want the default", got)
	}
}

// Drain is idempotent and safe before Run: a stop ceremony nobody
// started must not panic on the way out (FR-011).
func TestDrainIsIdempotentAndSafeBeforeRun(t *testing.T) {
	d := &Dispatcher{}
	if err := d.Drain(context.Background()); err != nil {
		t.Fatalf("drain before run: %v", err)
	}
	if err := d.Drain(context.Background()); err != nil {
		t.Fatalf("second drain: %v", err)
	}
	if d.acceptingWork() {
		t.Fatal("a drained dispatcher still takes work on")
	}
}

// Drain cancels every engine and waits for it, so an in-flight harness
// failure gets to report itself before the process ends (FR-011).
func TestDrainCancelsAndWaits(t *testing.T) {
	d := &Dispatcher{}
	d.taking = true
	d.served = map[string]*serve{}
	d.backoff = map[string]time.Time{}

	ctx, cancel := context.WithCancel(context.Background())
	s := &serve{persona: "clerk", cancel: cancel, done: make(chan struct{})}
	if !d.take("w-1", s) {
		t.Fatal("take refused a free slot")
	}
	stopped := make(chan struct{})
	go func() {
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond) // the self-report the drain waits for
		close(stopped)
		close(s.done)
	}()
	if err := d.Drain(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("Drain returned before the engine finished")
	}
	if d.take("w-1", &serve{done: make(chan struct{})}) {
		t.Fatal("a drained dispatcher took a placement on")
	}
}

// A drain that cannot finish inside its bound says so rather than
// hanging the process.
func TestDrainReportsAnUnfinishedWait(t *testing.T) {
	d := &Dispatcher{}
	d.taking = true
	d.served = map[string]*serve{"w-1": {persona: "clerk", cancel: func() {}, done: make(chan struct{})}}
	d.backoff = map[string]time.Time{}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := d.Drain(ctx)
	if err == nil || !strings.Contains(err.Error(), "drain did not finish") {
		t.Fatalf("Drain() = %v, want the unfinished-drain error", err)
	}
}

// Self-selection is what keeps a node from claiming work it cannot run:
// a placement it already serves, one it just failed at, and a persona it
// already has an engine for are all simply not raced (FR-008, the
// duplicate-persona edge case).
func TestSelfSelection(t *testing.T) {
	d := &Dispatcher{RaceBackoff: time.Hour}
	d.taking = true
	d.served = map[string]*serve{}
	d.backoff = map[string]time.Time{}

	if d.skipRace("w-1", "clerk") {
		t.Fatal("a fresh placement was skipped")
	}
	d.served["w-1"] = &serve{persona: "clerk", cancel: func() {}, done: make(chan struct{})}
	if !d.skipRace("w-1", "clerk") {
		t.Error("a placement this node already serves was raced again")
	}
	if !d.skipRace("w-2", "clerk") {
		t.Error("a second placement for a busy persona was raced")
	}
	if d.skipRace("w-2", "other") {
		t.Error("a different persona was skipped")
	}
	if free, why := d.slotFree("w-2", "clerk"); free || why == "" {
		t.Errorf("slotFree(busy persona) = %v %q, want a loud refusal", free, why)
	}

	d.backOff("w-3")
	if !d.skipRace("w-3", "other") {
		t.Error("a backed-off placement was re-raced")
	}

	// An engine that stops on its own leaves the served set — the probe
	// answers stop with it, so a peer's sweep can reclaim the placement.
	d.release("w-1")
	if _, still := d.served["w-1"]; still {
		t.Error("a released placement stayed in the served set")
	}
	if !d.skipRace("w-1", "clerk") {
		t.Error("a just-released placement was immediately re-raced")
	}
}

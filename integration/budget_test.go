package integration

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/internal/natstest"
	"github.com/impire-io/soulstream-workloads/wrap"
)

// budgetRig is a hermetic realm with an owner and any number of wrapped
// agents — the 008 integration shape, ported from the research rig that
// measured the mechanism (episode 0128).
type budgetRig struct {
	url     string
	clients map[string]*realm.Client
}

func startBudgetRealm(t *testing.T, personas ...string) (*budgetRig, string) {
	t.Helper()
	url, shutdown := natstest.StartJetStream(t)
	t.Cleanup(shutdown)
	ctx := context.Background()
	rig := &budgetRig{url: url, clients: map[string]*realm.Client{}}
	for _, p := range personas {
		nc, err := nats.Connect(url)
		if err != nil {
			t.Fatalf("connect %s: %v", p, err)
		}
		c, err := realm.NewClient(ctx, nc, realm.Config{Realm: "test-realm", Persona: p})
		if err != nil {
			t.Fatalf("client %s: %v", p, err)
		}
		t.Cleanup(func() { _ = c.Close() })
		rig.clients[p] = c
	}
	if _, err := rig.clients[personas[0]].Provision(ctx); err != nil {
		t.Fatalf("provision: %v", err)
	}
	h, err := topic.StartTopic(ctx, rig.clients[personas[0]], topic.StartTopicInput{Name: "budget"})
	if err != nil {
		t.Fatalf("start topic: %v", err)
	}
	return rig, h.Path()
}

func (r *budgetRig) post(t *testing.T, persona, path, body string) {
	t.Helper()
	if _, err := topic.Open(r.clients[persona], path).PostTurn(context.Background(), body); err != nil {
		t.Fatalf("post as %s: %v", persona, err)
	}
}

func (r *budgetRig) agentTurns(t *testing.T, path string, agents ...string) []topic.Contribution {
	t.Helper()
	view, err := topic.Open(r.clients["owner"], path).Materialise(context.Background())
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}
	isAgent := map[string]bool{}
	for _, a := range agents {
		isAgent[a] = true
	}
	var out []topic.Contribution
	for _, c := range view.Contributions {
		if c.Type == "turn.post" && isAgent[c.Author] {
			out = append(out, c)
		}
	}
	return out
}

// settle waits until the topic's agent-turn count is stable for quiet (or
// max elapses) and returns the final count and whether it settled.
func (r *budgetRig) settle(t *testing.T, path string, quiet, limit time.Duration, agents ...string) (int, bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	last, lastChange := -1, time.Now()
	for time.Now().Before(deadline) {
		n := len(r.agentTurns(t, path, agents...))
		if n != last {
			last, lastChange = n, time.Now()
		} else if time.Since(lastChange) >= quiet {
			return n, true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return len(r.agentTurns(t, path, agents...)), false
}

// refusalCounter counts wake_refused and wrap_unbudgeted log records;
// everything else is discarded.
type refusalCounter struct{ n, unbudgeted atomic.Int64 }

func (c *refusalCounter) Enabled(context.Context, slog.Level) bool { return true }
func (c *refusalCounter) Handle(_ context.Context, r slog.Record) error {
	switch r.Message {
	case "wake_refused":
		c.n.Add(1)
	case "wrap_unbudgeted":
		c.unbudgeted.Add(1)
	}
	return nil
}
func (c *refusalCounter) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *refusalCounter) WithGroup(string) slog.Handler      { return c }

func budgetTemplate() wrap.Template {
	return wrap.Template{
		Command: []string{"/usr/bin/true"},
		Prompt:  "AUTHOR={{AUTHOR}}\nBODY={{BODY}}",
		Terminal: wrap.TerminalMap{
			TypeField: "type", TerminalValue: "result", TextField: "result",
		},
	}
}

func promptFields(prompt string) (author, body string) {
	for _, line := range strings.Split(prompt, "\n") {
		if v, ok := strings.CutPrefix(line, "AUTHOR="); ok {
			author = v
		}
		if v, ok := strings.CutPrefix(line, "BODY="); ok {
			body = v
		}
	}
	return
}

// runScriptWrapper starts a real wrap.Wrapper whose harness is the script
// closure; stop cancels and waits.
func runScriptWrapper(t *testing.T, rig *budgetRig, persona string, cfgMod func(*wrap.Config), script func(author, body string) (string, bool), counter *refusalCounter) (stop func()) {
	t.Helper()
	cfg := wrap.Config{Persona: persona, Template: budgetTemplate(), Scratch: t.TempDir(),
		RunTimeout: 5 * time.Second, Retries: 1}
	if cfgMod != nil {
		cfgMod(&cfg)
	}
	w := &wrap.Wrapper{
		Config: cfg,
		Client: rig.clients[persona],
		Invoke: func(_ context.Context, spec wrap.RunSpec) wrap.HarnessResult {
			author, body := promptFields(spec.Prompt)
			text, ok := script(author, body)
			if !ok {
				return wrap.HarnessResult{OK: false, Detail: "script refused"}
			}
			return wrap.HarnessResult{OK: true, Text: text, Detail: "scripted"}
		},
		Log: slog.New(counter),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = w.Run(ctx) }()
	time.Sleep(150 * time.Millisecond) // let the live subscription attach
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Log("wrapper did not stop in time")
		}
	}
}

// An uncooperative two-agent cycle — replies always mention the other —
// halts at exactly MaxHops outcomes with loud, op-less refusals (SC-001,
// SC-004; the rig measured 421 turns in 5s without the budget).
func TestBudgetHaltsUncooperativeCycle(t *testing.T) {
	rig, path := startBudgetRealm(t, "owner", "agent-a", "agent-b")
	counter := &refusalCounter{}
	const maxHops = 4
	budget := func(c *wrap.Config) {
		c.Budget = wrap.Budget{MaxHops: maxHops, WindowMax: 100, WindowPer: time.Hour}
	}
	stopA := runScriptWrapper(t, rig, "agent-a", budget, func(_, _ string) (string, bool) {
		return "ping @agent-b", true
	}, counter)
	defer stopA()
	stopB := runScriptWrapper(t, rig, "agent-b", budget, func(_, _ string) (string, bool) {
		return "pong @agent-a", true
	}, counter)
	defer stopB()

	rig.post(t, "owner", path, "@agent-a start")
	n, settled := rig.settle(t, path, 1500*time.Millisecond, 20*time.Second, "agent-a", "agent-b")

	if !settled {
		t.Fatalf("cascade did not settle under the depth budget (%d agent turns)", n)
	}
	if n != maxHops {
		t.Fatalf("agent turns = %d, want exactly MaxHops = %d", n, maxHops)
	}
	if counter.n.Load() < 1 {
		t.Fatal("expected at least one loud refusal")
	}
	// Op-less: no turn beyond the bound, and no refusal testimony either —
	// the count above is the whole record.
	for _, c := range rig.agentTurns(t, path, "agent-a", "agent-b") {
		if strings.Contains(c.Body, "could not answer") {
			t.Fatalf("a refusal posted testimony: %q", c.Body)
		}
	}
}

// The id-evading variant — agents post outcomes through their own client
// under arbitrary ids (the MCP arm), invisible to the depth walk — halts
// within 2×WindowMax on the window floor (SC-002; the rig measured 393
// turns in 3s past an evaded depth gate).
func TestBudgetWindowHaltsSelfPostCycle(t *testing.T) {
	rig, path := startBudgetRealm(t, "owner", "agent-a", "agent-b")
	counter := &refusalCounter{}
	const windowMax = 3
	budget := func(c *wrap.Config) {
		c.Budget = wrap.Budget{MaxHops: 4, WindowMax: windowMax, WindowPer: time.Hour}
	}
	selfPost := func(persona, text string) func(author, body string) (string, bool) {
		return func(_, _ string) (string, bool) {
			// The harness posts as itself mid-run; the wrapper correlates
			// the self-post and posts nothing.
			_, err := topic.Open(rig.clients[persona], path).PostTurn(context.Background(), text)
			if err != nil {
				return "", false
			}
			return "already posted myself", true
		}
	}
	stopA := runScriptWrapper(t, rig, "agent-a", budget, selfPost("agent-a", "ping @agent-b"), counter)
	defer stopA()
	stopB := runScriptWrapper(t, rig, "agent-b", budget, selfPost("agent-b", "pong @agent-a"), counter)
	defer stopB()

	rig.post(t, "owner", path, "@agent-a start")
	n, settled := rig.settle(t, path, 1500*time.Millisecond, 20*time.Second, "agent-a", "agent-b")

	if !settled {
		t.Fatalf("self-post cascade did not settle under the window budget (%d agent turns)", n)
	}
	if n > 2*windowMax {
		t.Fatalf("agent turns = %d, want at most 2K = %d", n, 2*windowMax)
	}
	if counter.n.Load() < 1 {
		t.Fatal("expected at least one loud refusal")
	}
}

// Legitimate delegation under the DEFAULT budget: owner→A→B→A completes
// with exactly three outcomes and zero refusals (SC-003).
func TestBudgetDelegationSurvivesDefaults(t *testing.T) {
	rig, path := startBudgetRealm(t, "owner", "agent-a", "agent-b")
	counter := &refusalCounter{}
	stopA := runScriptWrapper(t, rig, "agent-a", nil, func(_, body string) (string, bool) {
		switch {
		case strings.Contains(body, "please summarise"):
			return "on it — @agent-b fetch the data", true
		case strings.Contains(body, "data:"):
			return "done: sunny, 24C", true
		}
		return "", false
	}, counter)
	defer stopA()
	stopB := runScriptWrapper(t, rig, "agent-b", nil, func(_, body string) (string, bool) {
		if strings.Contains(body, "fetch the data") {
			return "@agent-a data: sunny, 24C", true
		}
		return "", false
	}, counter)
	defer stopB()

	rig.post(t, "owner", path, "@agent-a please summarise the weather")
	n, settled := rig.settle(t, path, 1500*time.Millisecond, 20*time.Second, "agent-a", "agent-b")

	if !settled || n != 3 {
		t.Fatalf("delegation: settled=%v agent turns=%d, want settled with exactly 3", settled, n)
	}
	if got := counter.n.Load(); got != 0 {
		t.Fatalf("refusals = %d inside a legitimate chain, want 0", got)
	}
}

// Opting out is explicit and reproduces the unbounded wrapper: the same
// uncooperative cycle runs well past any default bound (SC-005 behavioral
// half; the config half is unit-tested).
func TestBudgetUnbudgetedRunsUnbounded(t *testing.T) {
	rig, path := startBudgetRealm(t, "owner", "agent-a", "agent-b")
	counter := &refusalCounter{}
	unbudgeted := func(c *wrap.Config) { c.Unbudgeted = true }
	stopA := runScriptWrapper(t, rig, "agent-a", unbudgeted, func(_, _ string) (string, bool) {
		return "ping @agent-b", true
	}, counter)
	stopB := runScriptWrapper(t, rig, "agent-b", unbudgeted, func(_, _ string) (string, bool) {
		return "pong @agent-a", true
	}, counter)

	rig.post(t, "owner", path, "@agent-a start")
	deadline := time.Now().Add(8 * time.Second)
	n := 0
	for time.Now().Before(deadline) {
		n = len(rig.agentTurns(t, path, "agent-a", "agent-b"))
		if n > 12 { // past every default bound (MaxHops=4, 2K=16 not reached but 12 > 4 and growing)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	stopA()
	stopB()
	if n <= 12 {
		t.Fatalf("unbudgeted cycle produced only %d turns — opt-out did not reproduce the unbounded wrapper", n)
	}
	if counter.n.Load() != 0 {
		t.Fatalf("unbudgeted wrapper refused %d wakes", counter.n.Load())
	}
	if got := counter.unbudgeted.Load(); got != 2 {
		t.Fatalf("wrap_unbudgeted logged %d times, want once per wrapper (2)", got)
	}
}

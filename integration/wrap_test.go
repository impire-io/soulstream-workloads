package integration

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/internal/natstest"
	"github.com/impire-io/soulstream-workloads/wrap"
)

// wrapRig is one test's hermetic realm: an owner who mentions, and the
// wrapped agent's own client (the wrapper's whole standing).
type wrapRig struct {
	url   string
	owner *realm.Client
	agent *realm.Client
}

func startWrapRealm(t *testing.T) (*wrapRig, string) {
	t.Helper()
	url, shutdown := natstest.StartJetStream(t)
	t.Cleanup(shutdown)

	ctx := context.Background()
	connect := func(persona string) *realm.Client {
		nc, err := nats.Connect(url)
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		c, err := realm.NewClient(ctx, nc, realm.Config{Realm: "test-realm", Persona: persona})
		if err != nil {
			t.Fatalf("client %s: %v", persona, err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	owner := connect("owner")
	if _, err := owner.Provision(ctx); err != nil {
		t.Fatalf("provision: %v", err)
	}
	h, err := topic.StartTopic(ctx, owner, topic.StartTopicInput{Name: "wraps"})
	if err != nil {
		t.Fatalf("start topic: %v", err)
	}
	return &wrapRig{url: url, owner: owner, agent: connect("clerk")}, h.Path()
}

func mockTemplate(t *testing.T, mockBin, grammar, reply, mode, url string) wrap.Template {
	t.Helper()
	terminal := wrap.TerminalMap{TypeField: "type", TerminalValue: "result",
		TextField: "result", StatusField: "subtype", SuccessValue: "success"}
	if grammar == "codex" {
		terminal = wrap.TerminalMap{TypeField: "msg.type", TerminalValue: "task_complete",
			TextField: "msg.last_agent_message"}
	}
	return wrap.Template{
		Command: []string{mockBin, "--grammar", grammar, "--mode", mode, "--reply", reply,
			"--url", url, "--realm", "test-realm", "--persona", "clerk", "--topic", "{{TOPIC}}"},
		Prompt:   "You are @{{PERSONA}}; @{{AUTHOR}} said: {{BODY}}",
		Terminal: terminal,
	}
}

// runWrapper starts the wrapper over the agent's client and returns a stop
// function that waits for it to come down.
func runWrapper(t *testing.T, rig *wrapRig, tpl wrap.Template, timeout time.Duration, retries int) func() {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	if testing.Verbose() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	w := &wrap.Wrapper{
		Config: wrap.Config{Persona: "clerk", Template: tpl, Scratch: t.TempDir(),
			RunTimeout: timeout, Retries: retries},
		Client: rig.agent,
		Log:    logger,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = w.Run(ctx) }()
	stop := func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("wrapper did not stop")
		}
	}
	t.Cleanup(stop)
	return stop
}

func turnsBy(t *testing.T, c *realm.Client, path, author string) []topic.Contribution {
	t.Helper()
	view, err := topic.Open(c, path).Materialise(context.Background())
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}
	var out []topic.Contribution
	for _, contrib := range view.Contributions {
		if contrib.Author == author && contrib.Type == "turn.post" {
			out = append(out, contrib)
		}
	}
	return out
}

func waitTurns(t *testing.T, c *realm.Client, path, author string, want int) []topic.Contribution {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		got := turnsBy(t, c, path, author)
		if len(got) >= want {
			time.Sleep(1500 * time.Millisecond) // hold a beat: "exactly" means none extra
			got = turnsBy(t, c, path, author)
			if len(got) != want {
				t.Fatalf("turns by %s = %d, want exactly %d", author, len(got), want)
			}
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("turns by %s = %d after deadline, want %d", author, len(got), want)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func post(t *testing.T, c *realm.Client, path, body string) string {
	t.Helper()
	opID, err := topic.Open(c, path).PostTurn(context.Background(), body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return opID
}

// SC-001: mentions posted while nothing runs are answered on start (two in
// ONE topic — the measured multi-mention regression), a live mention is
// answered without restart, and a restart answers nothing twice.
func TestWrapAnswersBacklogLiveAndNeverTwice(t *testing.T) {
	rig, path := startWrapRealm(t)
	mock := buildCmd(t, "github.com/impire-io/soulstream-workloads/cmd/harness-mock")
	m1 := post(t, rig.owner, path, "@clerk backlog one")
	post(t, rig.owner, path, "@clerk backlog two")

	stop := runWrapper(t, rig, mockTemplate(t, mock, "claude", "answered.", "ok", rig.url), 30*time.Second, 2)
	replies := waitTurns(t, rig.owner, path, "clerk", 2)
	if replies[0].OpID != wrap.WakeOpID(m1, "clerk") {
		t.Fatalf("first reply op = %s, want the deterministic wake id", replies[0].OpID)
	}

	post(t, rig.owner, path, "@clerk and one while you are up")
	waitTurns(t, rig.owner, path, "clerk", 3)

	stop()
	runWrapper(t, rig, mockTemplate(t, mock, "claude", "answered.", "ok", rig.url), 30*time.Second, 2)
	time.Sleep(3 * time.Second) // catch-up runs; nothing new may appear
	if got := turnsBy(t, rig.owner, path, "clerk"); len(got) != 3 {
		t.Fatalf("after restart turns = %d, want still 3 — something answered twice", len(got))
	}
}

// SC-002: die and hang end in exactly one agent-authored self-report tapping
// only the asker; a mid-run self-post stands alone.
func TestWrapFaultsSelfReport(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		timeout time.Duration
		wantIn  string
	}{
		{"die at budget", "die", 30 * time.Second, "could not answer"},
		{"hang past budget", "hang", 2 * time.Second, "run timeout"},
		{"self-post correlated", "self-post", 30 * time.Second, "my own reply"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig, path := startWrapRealm(t)
			mock := buildCmd(t, "github.com/impire-io/soulstream-workloads/cmd/harness-mock")
			mention := post(t, rig.owner, path, "@clerk please answer")
			runWrapper(t, rig, mockTemplate(t, mock, "claude", "my own reply", tc.mode, rig.url), tc.timeout, 2)

			outcome := waitTurns(t, rig.owner, path, "clerk", 1)
			if !strings.Contains(outcome[0].Body, tc.wantIn) {
				t.Fatalf("outcome body = %q, want it to mention %q", outcome[0].Body, tc.wantIn)
			}
			if tc.mode != "self-post" {
				if outcome[0].OpID != wrap.WakeOpID(mention, "clerk") {
					t.Fatalf("self-report op = %s, want the wake id", outcome[0].OpID)
				}
				taps := strings.Join(outcome[0].Mentions, ",")
				if !strings.Contains(taps, "owner") || strings.Contains(taps, "clerk") {
					t.Fatalf("self-report taps = %v, want the asker and never the agent", outcome[0].Mentions)
				}
			}
		})
	}
}

// SC-003: an enforcing server refuses a credential-less agent loudly — the
// wrapper's connection is the admission check, and nothing gets posted.
func TestWrapRefusedLoudly(t *testing.T) {
	op := natstest.StartOperator(t)
	t.Cleanup(op.Shutdown)
	_, err := realm.Connect(context.Background(), realm.Config{
		URL: op.URL, Realm: "test-realm", Persona: "clerk",
	})
	if err == nil {
		t.Fatal("an operator-mode server admitted a credential-less agent")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "authorization") {
		t.Fatalf("refusal = %v, want the server's authorization voice", err)
	}
}

// SC-004: a structurally different grammar is a template change; the engine
// is byte-identical.
func TestWrapSecondGrammar(t *testing.T) {
	rig, path := startWrapRealm(t)
	mock := buildCmd(t, "github.com/impire-io/soulstream-workloads/cmd/harness-mock")
	post(t, rig.owner, path, "@clerk answer through the other grammar")
	runWrapper(t, rig, mockTemplate(t, mock, "codex", "codex-shaped reply", "ok", rig.url), 30*time.Second, 2)
	replies := waitTurns(t, rig.owner, path, "clerk", 1)
	if replies[0].Body != "codex-shaped reply" {
		t.Fatalf("reply = %q", replies[0].Body)
	}
}

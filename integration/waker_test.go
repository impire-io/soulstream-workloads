package integration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/internal/natstest"
	"github.com/impire-io/soulstream-workloads/waker"
)

// wakerRig is one test's hermetic realm plus a waker serving it.
type wakerRig struct {
	url    string
	owner  *realm.Client // posts mentions, materialises for assertions
	wclank *realm.Client // the waker's own client
	cancel context.CancelFunc
	done   chan error
}

func startWakerRealm(t *testing.T) (*wakerRig, string) {
	t.Helper()
	url, shutdown := natstest.StartJetStream(t)
	t.Cleanup(shutdown)

	ctx := context.Background()
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	owner, err := realm.NewClient(ctx, nc, realm.Config{Realm: "test-realm", Persona: "owner"})
	if err != nil {
		t.Fatalf("owner client: %v", err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	if _, err := owner.Provision(ctx); err != nil {
		t.Fatalf("provision: %v", err)
	}
	h, err := topic.StartTopic(ctx, owner, topic.StartTopicInput{Name: "wakes"})
	if err != nil {
		t.Fatalf("start topic: %v", err)
	}

	ncw, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("waker connect: %v", err)
	}
	wclient, err := realm.NewClient(ctx, ncw, realm.Config{Realm: "test-realm", Persona: "the-waker"})
	if err != nil {
		t.Fatalf("waker client: %v", err)
	}
	t.Cleanup(func() { _ = wclient.Close() })

	return &wakerRig{url: url, owner: owner, wclank: wclient}, h.Path()
}

// serve starts the waker over cfg; the backlog already in the stream is what
// it drains. Stopped via t.Cleanup.
func (r *wakerRig) serve(t *testing.T, cfg waker.Config) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.done = make(chan error, 1)
	logger := slog.New(slog.DiscardHandler)
	if testing.Verbose() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	w := &waker.Waker{Config: cfg, Client: r.wclank, Log: logger}
	go func() { r.done <- w.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-r.done:
		case <-time.After(10 * time.Second):
			t.Error("waker did not stop")
		}
	})
}

func (r *wakerRig) config(t *testing.T, agents ...waker.Registration) waker.Config {
	t.Helper()
	return waker.Config{
		Waker: waker.Identity{Context: "unused-in-process", Realm: "test-realm",
			Persona: "the-waker", Scratch: t.TempDir()},
		Agents: agents,
	}
}

func claudeMockReg(url, mockBin, persona, reply, mode string, timeout time.Duration) waker.Registration {
	return waker.Registration{
		Persona:    persona,
		Credential: waker.Credential{URL: url},
		MaxDeliver: 2,
		RunTimeout: waker.Duration(timeout),
		Template: waker.Template{
			Command: []string{mockBin, "--grammar", "claude", "--mode", mode, "--reply", reply,
				"--url", url, "--realm", "test-realm", "--persona", persona, "--topic", "{{TOPIC}}"},
			Prompt: "You are @{{PERSONA}}; @{{AUTHOR}} said: {{BODY}}",
			Terminal: waker.TerminalMap{TypeField: "type", TerminalValue: "result",
				TextField: "result", StatusField: "subtype", SuccessValue: "success"},
		},
	}
}

// turnsBy returns the topic's turn.post ops by author, and all ops.
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

// waitTurns polls until author has exactly want turns (and fails if more
// appear before the deadline settles).
func waitTurns(t *testing.T, c *realm.Client, path, author string, want int) []topic.Contribution {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		got := turnsBy(t, c, path, author)
		if len(got) >= want {
			// Hold a beat and re-check: "exactly one" means none extra arrive.
			time.Sleep(1500 * time.Millisecond)
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

func assertConsumerDrained(t *testing.T, url, persona string) {
	t.Helper()
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		cons, err := js.Consumer(context.Background(), realm.NotifyStreamName, "waker-"+persona)
		if err != nil {
			t.Fatalf("consumer: %v", err)
		}
		info, err := cons.Info(context.Background())
		if err != nil {
			t.Fatalf("consumer info: %v", err)
		}
		if info.NumPending == 0 && info.NumAckPending == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("consumer not drained: pending=%d ack_pending=%d", info.NumPending, info.NumAckPending)
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

// SC-001 / US1: a mention posted while no waker runs wakes the harness once
// the waker starts; exactly one reply lands, authored by the agent, under the
// wake's deterministic op id — the address outlives the process.
func TestWakerMentionWakesAgent(t *testing.T) {
	rig, path := startWakerRealm(t)
	mock := buildCmd(t, "github.com/impire-io/soulstream-workloads/cmd/harness-mock")

	mention := post(t, rig.owner, path, "Hello @clerk — anyone home?")
	rig.serve(t, rig.config(t, claudeMockReg(rig.url, mock, "clerk", "I am awake now.", "ok", 30*time.Second)))

	replies := waitTurns(t, rig.owner, path, "clerk", 1)
	if replies[0].Body != "I am awake now." {
		t.Fatalf("reply body = %q", replies[0].Body)
	}
	if replies[0].OpID != waker.WakeOpID(mention, "clerk") {
		t.Fatalf("reply op = %s, want the deterministic wake id %s", replies[0].OpID, waker.WakeOpID(mention, "clerk"))
	}
	assertConsumerDrained(t, rig.url, "clerk")
}

// US2: the three measured faults each end in exactly one outcome op. Die and
// hang produce the WAKER's testimony (never the agent's voice); a self-posted
// reply stands alone.
func TestWakerFaultsYieldOneOutcome(t *testing.T) {
	cases := []struct {
		name       string
		mode       string
		timeout    time.Duration
		wantAuthor string
		wantIn     string
	}{
		{"die at budget", "die", 30 * time.Second, "the-waker", "could not answer"},
		{"hang past budget", "hang", 2 * time.Second, "the-waker", "run timeout"},
		{"self-post correlated", "self-post", 30 * time.Second, "clerk", "my own reply"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig, path := startWakerRealm(t)
			mock := buildCmd(t, "github.com/impire-io/soulstream-workloads/cmd/harness-mock")

			mention := post(t, rig.owner, path, "@clerk please answer")
			rig.serve(t, rig.config(t, claudeMockReg(rig.url, mock, "clerk", "my own reply", tc.mode, tc.timeout)))

			outcome := waitTurns(t, rig.owner, path, tc.wantAuthor, 1)
			if !strings.Contains(outcome[0].Body, tc.wantIn) {
				t.Fatalf("outcome body = %q, want it to mention %q", outcome[0].Body, tc.wantIn)
			}
			if tc.wantAuthor == "the-waker" {
				// Failure rides the wake's one outcome slot, NAMES the agent
				// in the body, and taps only the asker — tapping the agent
				// would wake it to hear of its own failure (the measured
				// loop this gate caught).
				if outcome[0].OpID != waker.WakeOpID(mention, "clerk") {
					t.Fatalf("failure op = %s, want wake id", outcome[0].OpID)
				}
				if got := turnsBy(t, rig.owner, path, "clerk"); len(got) != 0 {
					t.Fatalf("agent authored %d turns during a failed wake, want 0", len(got))
				}
				if !strings.Contains(outcome[0].Body, "clerk") {
					t.Fatalf("failure body = %q, want the agent named", outcome[0].Body)
				}
				taps := strings.Join(outcome[0].Mentions, ",")
				if !strings.Contains(taps, "owner") || strings.Contains(taps, "clerk") {
					t.Fatalf("failure turn taps = %v, want the asker tapped and the agent not", outcome[0].Mentions)
				}
			}
			assertConsumerDrained(t, rig.url, "clerk")
		})
	}
}

// US3: mentions accumulate while nothing runs and all drain — three mentions
// in ONE topic, the measured multi-mention trap (research episode 0082).
func TestWakerBacklogDrains(t *testing.T) {
	rig, path := startWakerRealm(t)
	mock := buildCmd(t, "github.com/impire-io/soulstream-workloads/cmd/harness-mock")

	for i := 1; i <= 3; i++ {
		post(t, rig.owner, path, fmt.Sprintf("@clerk backlog question %d", i))
	}
	rig.serve(t, rig.config(t, claudeMockReg(rig.url, mock, "clerk", "answered.", "ok", 30*time.Second)))

	waitTurns(t, rig.owner, path, "clerk", 3)
	assertConsumerDrained(t, rig.url, "clerk")
}

// US4 / SC-004: two agents, two structurally different grammars, one waker
// process — the difference is the template alone.
func TestWakerSecondGrammarByTemplate(t *testing.T) {
	rig, path := startWakerRealm(t)
	mock := buildCmd(t, "github.com/impire-io/soulstream-workloads/cmd/harness-mock")

	codexReg := waker.Registration{
		Persona:    "scribe",
		Credential: waker.Credential{URL: rig.url},
		MaxDeliver: 2,
		RunTimeout: waker.Duration(30 * time.Second),
		Template: waker.Template{
			Command: []string{mock, "--grammar", "codex", "--mode", "ok", "--reply", "codex-shaped reply"},
			Prompt:  "@{{AUTHOR}}: {{BODY}}",
			Terminal: waker.TerminalMap{TypeField: "msg.type", TerminalValue: "task_complete",
				TextField: "msg.last_agent_message"},
		},
	}
	post(t, rig.owner, path, "@clerk and also @scribe — both of you, please")
	rig.serve(t, rig.config(t,
		claudeMockReg(rig.url, mock, "clerk", "claude-shaped reply", "ok", 30*time.Second),
		codexReg))

	clerk := waitTurns(t, rig.owner, path, "clerk", 1)
	scribe := waitTurns(t, rig.owner, path, "scribe", 1)
	if clerk[0].Body != "claude-shaped reply" || scribe[0].Body != "codex-shaped reply" {
		t.Fatalf("replies = %q / %q", clerk[0].Body, scribe[0].Body)
	}
}

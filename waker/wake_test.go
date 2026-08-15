package waker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/soulstream-core/record"
	"github.com/impire-io/soulstream-core/topic"
)

// The fakes: every seam records into one shared call list so the protocol's
// order is assertable as a string (the runner_test.go pattern).

type recorder struct{ calls []string }

func (r *recorder) note(s string)  { r.calls = append(r.calls, s) }
func (r *recorder) String() string { return strings.Join(r.calls, ",") }

type fakeMsg struct {
	rec       *recorder
	data      []byte
	headers   nats.Header
	delivered uint64
}

func notifyMsg(rec *recorder, opID, author string, delivered uint64) *fakeMsg {
	h := nats.Header{}
	h.Set(record.HeaderType, topic.TypeMentionNotify)
	return &fakeMsg{rec: rec, delivered: delivered, headers: h,
		data: []byte(fmt.Sprintf(`{"topic":"t1","op_id":%q,"author":%q}`, opID, author))}
}

func (m *fakeMsg) Data() []byte         { return m.data }
func (m *fakeMsg) Headers() nats.Header { return m.headers }
func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: m.delivered}, nil
}
func (m *fakeMsg) Ack() error { m.rec.note("ack"); return nil }
func (m *fakeMsg) NakWithDelay(_ time.Duration) error {
	m.rec.note("nak")
	return nil
}

type fakeRealm struct {
	rec   *recorder
	view  []Turn
	after []Turn // returned once view was read (before/after evolution)
	reads int
}

func (f *fakeRealm) Read(_ context.Context, _ string) ([]Turn, error) {
	f.rec.note("read")
	f.reads++
	if f.after != nil && f.reads > 1 {
		return f.after, nil
	}
	return f.view, nil
}

func (f *fakeRealm) PostAsWaker(_ context.Context, _, _ string, mentions []string, opID string) (string, error) {
	f.rec.note("post_waker(" + strings.Join(mentions, "+") + ")")
	return opID, nil
}

type fakeSession struct{ rec *recorder }

func (s *fakeSession) Post(_ context.Context, _, _, opID string) (string, error) {
	s.rec.note("post_agent")
	return opID, nil
}
func (s *fakeSession) Close() { s.rec.note("close") }

func dialOK(rec *recorder) AgentDialer {
	return func(_ context.Context, _ string) (AgentSession, map[string]string, error) {
		rec.note("dial")
		return &fakeSession{rec: rec}, nil, nil
	}
}

func dialErr(rec *recorder, err error) AgentDialer {
	return func(_ context.Context, _ string) (AgentSession, map[string]string, error) {
		rec.note("dial")
		return nil, nil, err
	}
}

func invokeResult(rec *recorder, res HarnessResult) Invoker {
	return func(_ context.Context, _ RunSpec) HarnessResult {
		rec.note("invoke")
		return res
	}
}

func testReg() Registration {
	return Registration{
		Persona:    "clerk",
		MaxDeliver: 2,
		RunTimeout: Duration(5 * time.Second),
		Template: Template{Command: []string{"x"}, Prompt: "p",
			Terminal: TerminalMap{TypeField: "type", TerminalValue: "result", TextField: "result"}},
	}
}

func newHandler(_ *recorder, realm *fakeRealm, dial AgentDialer, inv Invoker) *handler {
	return &handler{reg: testReg(), realm: realm, dial: dial, invoke: inv,
		scratch: "/tmp", log: slog.New(slog.DiscardHandler)}
}

// The admitted happy path: dial (the probe), read, invoke, read, post as the
// agent, ack — in exactly that order, ack last.
func TestHandleHappyPath(t *testing.T) {
	rec := &recorder{}
	h := newHandler(rec, &fakeRealm{rec: rec}, dialOK(rec),
		invokeResult(rec, HarnessResult{OK: true, Text: "pong"}))
	h.handle(context.Background(), notifyMsg(rec, "m1", "owner", 1))
	want := "dial,read,invoke,read,post_agent,ack,close"
	if rec.String() != want {
		t.Fatalf("calls = %s, want %s", rec, want)
	}
}

// A refused dial refuses the wake: no harness, no post, no op — nak alone.
func TestHandleRefusedWake(t *testing.T) {
	rec := &recorder{}
	h := newHandler(rec, &fakeRealm{rec: rec},
		dialErr(rec, fmt.Errorf("%w: auth violation", ErrRefused)),
		invokeResult(rec, HarnessResult{OK: true, Text: "never"}))
	h.handle(context.Background(), notifyMsg(rec, "m1", "owner", 1))
	if rec.String() != "dial,nak" {
		t.Fatalf("calls = %s, want dial,nak", rec)
	}
}

// An unreachable realm is the transient class: same silence, sooner retry.
func TestHandleUnreachableWake(t *testing.T) {
	rec := &recorder{}
	h := newHandler(rec, &fakeRealm{rec: rec},
		dialErr(rec, fmt.Errorf("connection refused")),
		invokeResult(rec, HarnessResult{OK: true, Text: "never"}))
	h.handle(context.Background(), notifyMsg(rec, "m1", "owner", 1))
	if rec.String() != "dial,nak" {
		t.Fatalf("calls = %s, want dial,nak", rec)
	}
}

// A failed run under budget retries; at budget it posts the failure turn in
// the WAKER's voice, mentioning the agent and the asker, then acks.
func TestHandleFailureBudget(t *testing.T) {
	rec := &recorder{}
	h := newHandler(rec, &fakeRealm{rec: rec}, dialOK(rec),
		invokeResult(rec, HarnessResult{Detail: "harness died"}))
	h.handle(context.Background(), notifyMsg(rec, "m1", "owner", 1))
	if rec.String() != "dial,read,invoke,read,nak,close" {
		t.Fatalf("under budget: calls = %s", rec)
	}

	rec2 := &recorder{}
	h2 := newHandler(rec2, &fakeRealm{rec: rec2}, dialOK(rec2),
		invokeResult(rec2, HarnessResult{Detail: "harness died"}))
	h2.handle(context.Background(), notifyMsg(rec2, "m1", "owner", 2))
	// Delivery 2 runs the redelivery pre-check read before anything else, and
	// the failure turn taps the asker alone — never the agent, which would
	// wake it to hear of its own failure (the measured loop).
	want := "read,dial,read,invoke,read,post_waker(owner),ack,close"
	if rec2.String() != want {
		t.Fatalf("at budget: calls = %s, want %s", rec2, want)
	}
}

// An agent's own mention of itself is acked, never woken — the self-loop
// guard the integration gate's measured runaway forced.
func TestHandleSelfMentionGuard(t *testing.T) {
	rec := &recorder{}
	h := newHandler(rec, &fakeRealm{rec: rec}, dialOK(rec),
		invokeResult(rec, HarnessResult{OK: true, Text: "never"}))
	h.handle(context.Background(), notifyMsg(rec, "m1", "clerk", 1))
	if rec.String() != "ack" {
		t.Fatalf("calls = %s, want ack alone", rec)
	}
}

// A harness that posted its own reply during the run is correlated: the waker
// posts nothing and acks.
func TestHandleCorrelatedSelfPost(t *testing.T) {
	rec := &recorder{}
	realm := &fakeRealm{rec: rec,
		view: []Turn{{OpID: "m1", Author: "owner", Type: "turn.post", Body: "hi @clerk"}},
		after: []Turn{{OpID: "m1", Author: "owner", Type: "turn.post", Body: "hi @clerk"},
			{OpID: "self", Author: "clerk", Type: "turn.post", Body: "did it myself"}},
	}
	h := newHandler(rec, realm, dialOK(rec),
		invokeResult(rec, HarnessResult{OK: true, Text: "I posted my reply in the topic."}))
	h.handle(context.Background(), notifyMsg(rec, "m1", "owner", 1))
	want := "dial,read,invoke,read,ack,close"
	if rec.String() != want {
		t.Fatalf("calls = %s, want %s", rec, want)
	}
}

// The redelivery pre-check: an outcome that already landed is acked without
// probing or invoking anything.
func TestHandleRedeliveryPreCheck(t *testing.T) {
	rec := &recorder{}
	realm := &fakeRealm{rec: rec,
		view: []Turn{{OpID: WakeOpID("m1", "clerk"), Author: "clerk", Type: "turn.post", Body: "already answered"}}}
	h := newHandler(rec, realm, dialOK(rec),
		invokeResult(rec, HarnessResult{OK: true, Text: "never"}))
	h.handle(context.Background(), notifyMsg(rec, "m1", "owner", 2))
	if rec.String() != "read,ack" {
		t.Fatalf("calls = %s, want read,ack", rec)
	}
}

// A notify that is not a mention is acked and never becomes a wake.
func TestHandleSkipsForeignNotifyTypes(t *testing.T) {
	rec := &recorder{}
	h := newHandler(rec, &fakeRealm{rec: rec}, dialOK(rec),
		invokeResult(rec, HarnessResult{OK: true, Text: "never"}))
	msg := notifyMsg(rec, "m1", "owner", 1)
	msg.headers.Set(record.HeaderType, "presence.update")
	h.handle(context.Background(), msg)
	if rec.String() != "ack" {
		t.Fatalf("calls = %s, want ack alone", rec)
	}
}

package wrap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type recorder struct{ calls []string }

func (r *recorder) note(s string)  { r.calls = append(r.calls, s) }
func (r *recorder) String() string { return strings.Join(r.calls, ",") }

type fakeRealm struct {
	rec   *recorder
	view  []Turn
	after []Turn // returned from the second read on (before/after evolution)
	reads int
	fail  bool
}

func (f *fakeRealm) Read(_ context.Context, _ string) ([]Turn, error) {
	f.rec.note("read")
	if f.fail {
		return nil, fmt.Errorf("no realm today")
	}
	f.reads++
	if f.after != nil && f.reads > 1 {
		return f.after, nil
	}
	return f.view, nil
}

func (f *fakeRealm) Post(_ context.Context, _, _ string, mentions []string, opID string) (string, error) {
	f.rec.note("post(" + strings.Join(mentions, "+") + ")")
	return opID, nil
}

func invokeResult(rec *recorder, res HarnessResult) Invoker {
	return func(_ context.Context, _ RunSpec) HarnessResult {
		rec.note("invoke")
		return res
	}
}

func testCfg(retries int) Config {
	return Config{
		Persona:    "clerk",
		Retries:    retries,
		RunTimeout: 5 * time.Second,
		Scratch:    "/tmp",
		Template: Template{Command: []string{"x"}, Prompt: "p",
			Terminal: TerminalMap{TypeField: "type", TerminalValue: "result", TextField: "result"}},
	}
}

func wake(author string) Wake { return Wake{Topic: "t1", OpID: "m1", Author: author} }

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

// The happy path: read (existence check doubles as the before snapshot),
// invoke, read, post as the agent under the deterministic id.
func TestWakeHappyPath(t *testing.T) {
	rec := &recorder{}
	got, err := handleWake(context.Background(), testCfg(1), &fakeRealm{rec: rec},
		invokeResult(rec, HarnessResult{OK: true, Text: "pong"}), wake("owner"), discard())
	if err != nil || got != "answered" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if rec.String() != "read,invoke,read,post()" {
		t.Fatalf("calls = %s", rec)
	}
}

// An outcome already in the record means answered — the restart/duplicate
// guard: no invoke, no post, at any redelivery distance.
func TestWakeAlreadyAnswered(t *testing.T) {
	rec := &recorder{}
	realm := &fakeRealm{rec: rec,
		view: []Turn{{OpID: WakeOpID("m1", "clerk"), Author: "clerk", Type: "turn.post"}}}
	got, err := handleWake(context.Background(), testCfg(1), realm,
		invokeResult(rec, HarnessResult{OK: true, Text: "never"}), wake("owner"), discard())
	if err != nil || got != "already_answered" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if rec.String() != "read" {
		t.Fatalf("calls = %s", rec)
	}
}

// The measured self-loop guard: an agent's own mention of itself never wakes
// it — not even a read.
func TestWakeSelfMentionSkipped(t *testing.T) {
	rec := &recorder{}
	got, err := handleWake(context.Background(), testCfg(1), &fakeRealm{rec: rec},
		invokeResult(rec, HarnessResult{OK: true, Text: "never"}), wake("clerk"), discard())
	if err != nil || got != "self_skipped" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if rec.String() != "" {
		t.Fatalf("calls = %s, want none", rec)
	}
}

// Spent retries end in the agent's self-report: its own voice, tapping only
// the asker, under the same one outcome id.
func TestWakeSelfReportAtBudget(t *testing.T) {
	rec := &recorder{}
	got, err := handleWake(context.Background(), testCfg(1), &fakeRealm{rec: rec},
		invokeResult(rec, HarnessResult{Detail: "harness died"}), wake("owner"), discard())
	if err != nil || got != "self_reported" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if rec.String() != "read,invoke,read,post(owner)" {
		t.Fatalf("calls = %s, want the asker tapped and nobody else", rec)
	}
}

// A harness that posted its own reply mid-run is correlated by the snapshot
// difference: the wrapper posts nothing.
func TestWakeCorrelatedSelfPost(t *testing.T) {
	rec := &recorder{}
	realm := &fakeRealm{rec: rec,
		view: []Turn{{OpID: "m1", Author: "owner", Type: "turn.post", Body: "hi @clerk"}},
		after: []Turn{{OpID: "m1", Author: "owner", Type: "turn.post", Body: "hi @clerk"},
			{OpID: "self", Author: "clerk", Type: "turn.post", Body: "did it myself"}}}
	got, err := handleWake(context.Background(), testCfg(1), realm,
		invokeResult(rec, HarnessResult{OK: true, Text: "I posted my reply."}), wake("owner"), discard())
	if err != nil || got != "answered" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if rec.String() != "read,invoke,read" {
		t.Fatalf("calls = %s, want no post", rec)
	}
}

// An unreachable realm parks the wake: an error, nothing posted — the next
// catch-up retries it and the deterministic id makes that safe.
func TestWakeUnreachableParks(t *testing.T) {
	rec := &recorder{}
	_, err := handleWake(context.Background(), testCfg(1), &fakeRealm{rec: rec, fail: true},
		invokeResult(rec, HarnessResult{OK: true, Text: "never"}), wake("owner"), discard())
	if err == nil {
		t.Fatal("want an error when the realm is unreachable")
	}
	if strings.Contains(rec.String(), "post") || strings.Contains(rec.String(), "invoke") {
		t.Fatalf("calls = %s, want neither invoke nor post", rec)
	}
}

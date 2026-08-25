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

// The wake budget refuses op-lessly: no invoke, no post, outcome
// "refused" — and the one log line carries the legible reason with the
// numbers (design 0006 §2: a refusal that posts is a wake source).
func TestWakeRefusedByWindowIsOpLessAndLoud(t *testing.T) {
	rec := &recorder{}
	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, nil))
	cfg := testCfg(1)
	cfg.Budget = Budget{WindowMax: 1, WindowPer: time.Hour}
	realm := &fakeRealm{rec: rec, view: []Turn{
		{OpID: "m1", Author: "owner", Type: "turn.post", Body: "hi @clerk", Timestamp: time.Now()},
		{OpID: "r0", Author: "clerk", Type: "turn.post", Body: "earlier answer", Timestamp: time.Now()},
	}}
	got, err := handleWake(context.Background(), cfg, realm,
		invokeResult(rec, HarnessResult{OK: true, Text: "never"}), wake("owner"), log)
	if err != nil || got != "refused" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if rec.String() != "read" {
		t.Fatalf("calls = %s, want the read and nothing else", rec)
	}
	logged := buf.String()
	if !strings.Contains(logged, "wake_refused") || !strings.Contains(logged, "window budget") ||
		!strings.Contains(logged, "bound 1") {
		t.Fatalf("refusal log not legible: %q", logged)
	}
}

// The depth bound refuses a wake whose outcome would overrun the provable
// chain — computed from the view, never from stream order.
func TestWakeRefusedByDepth(t *testing.T) {
	rec := &recorder{}
	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, nil))
	cfg := testCfg(1)
	cfg.Budget = Budget{MaxHops: 1}
	// The trigger is itself a provable wake outcome (hop 1): admitting
	// would put clerk's outcome at hop 2 > 1.
	triggerID := WakeOpID("m0", "bot")
	realm := &fakeRealm{rec: rec, view: []Turn{
		{OpID: "m0", Author: "owner", Type: "turn.post", Body: "hi @bot"},
		{OpID: triggerID, Author: "bot", Type: "turn.post", Body: "hi @clerk"},
	}}
	got, err := handleWake(context.Background(), cfg, realm,
		invokeResult(rec, HarnessResult{OK: true, Text: "never"}),
		Wake{Topic: "t1", OpID: triggerID, Author: "bot"}, log)
	if err != nil || got != "refused" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if rec.String() != "read" {
		t.Fatalf("calls = %s, want the read and nothing else", rec)
	}
	if !strings.Contains(buf.String(), "depth budget") {
		t.Fatalf("refusal log not legible: %q", buf.String())
	}
}

// Unbudgeted skips the gate entirely: a view that would refuse under any
// budget answers exactly as today.
func TestWakeUnbudgetedSkipsTheGate(t *testing.T) {
	rec := &recorder{}
	cfg := testCfg(1)
	cfg.Unbudgeted = true
	cfg.Budget = Budget{WindowMax: 1, WindowPer: time.Hour} // would refuse if consulted
	realm := &fakeRealm{rec: rec, view: []Turn{
		{OpID: "m1", Author: "owner", Type: "turn.post", Body: "hi @clerk", Timestamp: time.Now()},
		{OpID: "r0", Author: "clerk", Type: "turn.post", Body: "earlier answer", Timestamp: time.Now()},
	}}
	got, err := handleWake(context.Background(), cfg, realm,
		invokeResult(rec, HarnessResult{OK: true, Text: "pong"}), wake("owner"), discard())
	if err != nil || got != "answered" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if rec.String() != "read,invoke,read,post()" {
		t.Fatalf("calls = %s", rec)
	}
}

// A zero budget (no defaults applied — the raw config unit shape) refuses
// nothing: today's behavior byte-for-byte.
func TestWakeZeroBudgetIsTodaysBehavior(t *testing.T) {
	rec := &recorder{}
	realm := &fakeRealm{rec: rec, view: []Turn{
		{OpID: "m1", Author: "owner", Type: "turn.post", Body: "hi @clerk", Timestamp: time.Now()},
		{OpID: "r0", Author: "clerk", Type: "turn.post", Body: "earlier", Timestamp: time.Now()},
	}}
	got, err := handleWake(context.Background(), testCfg(1), realm,
		invokeResult(rec, HarnessResult{OK: true, Text: "pong"}), wake("owner"), discard())
	if err != nil || got != "answered" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if rec.String() != "read,invoke,read,post()" {
		t.Fatalf("calls = %s", rec)
	}
}

// --- 009: the wake engine's four kinds through the one seam ---

// promptSpy is an invoker that records the filled prompt it was handed.
func promptSpy(rec *recorder, res HarnessResult, prompt *string) Invoker {
	return func(_ context.Context, spec RunSpec) HarnessResult {
		rec.note("invoke")
		*prompt = spec.Prompt
		return res
	}
}

type fakeInstructions struct {
	text string
	err  error
}

func (f fakeInstructions) Materialise(context.Context) (string, error) { return f.text, f.err }

// A schedule wake: no author, body from the tick payload, outcome on the
// wake's (home) topic, and KIND filled for the prompt.
func TestWakeScheduleKindHappyPath(t *testing.T) {
	rec := &recorder{}
	var prompt string
	cfg := testCfg(1)
	cfg.Template.Prompt = "KIND={{KIND}}\nAUTHOR={{AUTHOR}}\nBODY={{BODY}}"
	w := Wake{Kind: KindSchedule, Topic: "home-topic", OpID: "137", Body: `schedule "daily" fired (@every 24h)`}
	got, err := handleWake(context.Background(), cfg, &fakeRealm{rec: rec},
		promptSpy(rec, HarnessResult{OK: true, Text: "did the rounds"}, &prompt), w, discard())
	if err != nil || got != "answered" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if rec.String() != "read,invoke,read,post()" {
		t.Fatalf("calls = %s", rec)
	}
	if !strings.Contains(prompt, "KIND=schedule") || !strings.Contains(prompt, `BODY=schedule "daily" fired`) ||
		!strings.Contains(prompt, "AUTHOR=\n") {
		t.Fatalf("prompt = %q", prompt)
	}
}

// A failed schedule/subject wake self-reports on the home topic tapping
// NOBODY — there is no asker (contract: wake-kinds.md).
func TestWakeNoAuthorSelfReportTapsNobody(t *testing.T) {
	for _, kind := range []WakeKind{KindSchedule, KindSubject} {
		rec := &recorder{}
		w := Wake{Kind: kind, Topic: "home-topic", OpID: "abc123", Body: "payload"}
		got, err := handleWake(context.Background(), testCfg(1), &fakeRealm{rec: rec},
			invokeResult(rec, HarnessResult{Detail: "harness died"}), w, discard())
		if err != nil || got != "self_reported" {
			t.Fatalf("%s outcome = %q, %v", kind, got, err)
		}
		if rec.String() != "read,invoke,read,post()" {
			t.Fatalf("%s calls = %s, want a tap-less self-report", kind, rec)
		}
	}
}

// A topic wake self-reports tapping the triggering op's author, and anchors
// its body from the view like a mention does.
func TestWakeTopicKindTapsTriggerAuthor(t *testing.T) {
	rec := &recorder{}
	var prompt string
	realm := &fakeRealm{rec: rec, view: []Turn{
		{OpID: "op1", Author: "poster", Type: "turn.post", Body: "fresh finding"},
	}}
	w := Wake{Kind: KindTopic, Topic: "watched", OpID: "op1", Author: "poster"}
	cfg := testCfg(1)
	cfg.Template.Prompt = "BODY={{BODY}}"
	got, err := handleWake(context.Background(), cfg, realm,
		promptSpy(rec, HarnessResult{Detail: "no answer"}, &prompt), w, discard())
	if err != nil || got != "self_reported" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if rec.String() != "read,invoke,read,post(poster)" {
		t.Fatalf("calls = %s, want the trigger author tapped", rec)
	}
	if !strings.Contains(prompt, "BODY=fresh finding") {
		t.Fatalf("prompt = %q, want the anchored body", prompt)
	}
}

// A topic wake for an op authored by the declared persona never wakes it —
// the normative self-exclusion at admission.
func TestWakeTopicKindSelfExcluded(t *testing.T) {
	rec := &recorder{}
	w := Wake{Kind: KindTopic, Topic: "watched", OpID: "op1", Author: "clerk"}
	got, err := handleWake(context.Background(), testCfg(1), &fakeRealm{rec: rec},
		invokeResult(rec, HarnessResult{OK: true, Text: "never"}), w, discard())
	if err != nil || got != "self_skipped" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if rec.String() != "" {
		t.Fatalf("calls = %s, want none", rec)
	}
}

// Declared instructions are materialised per wake and delivered through the
// prompt fill; legacy wakes fill KIND=mention.
func TestWakeInstructionsInPrompt(t *testing.T) {
	rec := &recorder{}
	var prompt string
	cfg := testCfg(1)
	cfg.Template.Prompt = "KIND={{KIND}}\nINSTRUCTIONS={{INSTRUCTIONS}}"
	cfg.Instructions = fakeInstructions{text: "v2: be thorough"}
	got, err := handleWake(context.Background(), cfg, &fakeRealm{rec: rec},
		promptSpy(rec, HarnessResult{OK: true, Text: "ok"}, &prompt), wake("owner"), discard())
	if err != nil || got != "answered" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if !strings.Contains(prompt, "INSTRUCTIONS=v2: be thorough") || !strings.Contains(prompt, "KIND=mention") {
		t.Fatalf("prompt = %q", prompt)
	}
}

// An instructions materialisation failure parks the wake loudly: an error,
// no invoke, no post — the trigger stays answerable and the agent never runs
// on unverifiable instructions.
func TestWakeInstructionsFailureParks(t *testing.T) {
	rec := &recorder{}
	cfg := testCfg(1)
	cfg.Instructions = fakeInstructions{err: fmt.Errorf("digest mismatch")}
	_, err := handleWake(context.Background(), cfg, &fakeRealm{rec: rec},
		invokeResult(rec, HarnessResult{OK: true, Text: "never"}), wake("owner"), discard())
	if err == nil || !strings.Contains(err.Error(), "instructions") {
		t.Fatalf("err = %v, want the instructions failure", err)
	}
	if strings.Contains(rec.String(), "invoke") || strings.Contains(rec.String(), "post") {
		t.Fatalf("calls = %s, want neither invoke nor post", rec)
	}
}

// The budget admits and refuses non-record kinds exactly like mentions: the
// window floor counts the persona's own turns on the home topic.
func TestWakeScheduleKindPassesBudgetGate(t *testing.T) {
	rec := &recorder{}
	cfg := testCfg(1)
	cfg.Budget = Budget{WindowMax: 1, WindowPer: time.Hour}
	realm := &fakeRealm{rec: rec, view: []Turn{
		{OpID: "r0", Author: "clerk", Type: "turn.post", Body: "earlier outcome", Timestamp: time.Now()},
	}}
	w := Wake{Kind: KindSchedule, Topic: "home-topic", OpID: "42", Body: "tick"}
	got, err := handleWake(context.Background(), cfg, realm,
		invokeResult(rec, HarnessResult{OK: true, Text: "never"}), w, discard())
	if err != nil || got != "refused" {
		t.Fatalf("outcome = %q, %v", got, err)
	}
	if rec.String() != "read" {
		t.Fatalf("calls = %s, want the read and nothing else", rec)
	}
}

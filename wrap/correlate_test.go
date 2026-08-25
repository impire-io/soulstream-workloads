package wrap

import (
	"testing"
	"time"
)

// The wake op-id is deterministic across redeliveries, UUID-shaped, and
// distinct per notify op.
func TestWakeOpIDDeterministic(t *testing.T) {
	a1 := WakeOpID("3febb367-2535-4855-912f-7996fbe32338", "clerk")
	a2 := WakeOpID("3febb367-2535-4855-912f-7996fbe32338", "clerk")
	b := WakeOpID("another-op", "clerk")
	if a1 != a2 {
		t.Fatalf("same notify op derived %q and %q, want identical", a1, a2)
	}
	if a1 == b {
		t.Fatal("distinct notify ops derived the same wake id")
	}
	if len(a1) != 36 {
		t.Fatalf("wake id %q is not UUID-shaped", a1)
	}
}

// The measured trap (research episode 0082): with several mentions in one
// topic, an earlier wake's reply must not satisfy a later wake. Snapshot
// difference is the correct primitive; this is its regression guard.
func TestPostedDuringRunIgnoresEarlierReplies(t *testing.T) {
	earlierReply := Turn{OpID: "r1", Author: "clerk", Type: "turn.post", Body: "one"}
	before := []Turn{
		{OpID: "m1", Author: "owner", Type: "turn.post"},
		{OpID: "m2", Author: "owner", Type: "turn.post"},
		earlierReply,
	}
	// Nothing new happened during this run.
	if op, ok := PostedDuringRun(before, before, "clerk"); ok {
		t.Fatalf("earlier reply %q satisfied a later wake", op)
	}
	// A genuinely new post during the run is found.
	after := append(append([]Turn{}, before...), Turn{OpID: "r2", Author: "clerk", Type: "turn.post"})
	op, ok := PostedDuringRun(before, after, "clerk")
	if !ok || op != "r2" {
		t.Fatalf("PostedDuringRun = %q,%v, want r2,true", op, ok)
	}
}

// Only the persona's own turn.post ops count as its reply.
func TestPostedDuringRunFiltersAuthorAndType(t *testing.T) {
	before := []Turn{}
	after := []Turn{
		{OpID: "x1", Author: "someone-else", Type: "turn.post"},
		{OpID: "x2", Author: "clerk", Type: "comment.add"},
	}
	if op, ok := PostedDuringRun(before, after, "clerk"); ok {
		t.Fatalf("foreign or non-turn op %q counted as the reply", op)
	}
}

// The redelivery pre-check and the anchor-body lookup.
func TestContainsOpAndAnchorBody(t *testing.T) {
	view := []Turn{{OpID: "a", Author: "owner", Type: "turn.post", Body: "hello @clerk"}}
	if !ContainsOp(view, "a") || ContainsOp(view, "b") {
		t.Fatal("ContainsOp misread the view")
	}
	if got := AnchorBody(view, "a"); got != "hello @clerk" {
		t.Fatalf("AnchorBody = %q", got)
	}
	if got := AnchorBody(view, "gone"); got != "" {
		t.Fatalf("AnchorBody for a rolled-up anchor = %q, want empty", got)
	}
}

// The walker resolves provable chains by the WakeOpID binding alone —
// never stream order — and reads unprovable ops as chain roots.
func TestWalkerResolvesProvableChains(t *testing.T) {
	root := Turn{OpID: "m1", Author: "owner", Type: "turn.post"}
	a1 := Turn{OpID: WakeOpID("m1", "agent-a"), Author: "agent-a", Type: "turn.post"}
	b1 := Turn{OpID: WakeOpID(a1.OpID, "agent-b"), Author: "agent-b", Type: "turn.post"}
	selfPosted := Turn{OpID: "arbitrary-id", Author: "agent-a", Type: "turn.post"}
	view := []Turn{root, a1, b1, selfPosted}

	for _, tc := range []struct {
		name     string
		op       Turn
		hops     int
		rootOpID string
	}{
		{"root is a root", root, 0, "m1"},
		{"first outcome", a1, 1, "m1"},
		{"second hop", b1, 2, "m1"},
		{"self-posted outcome reads as a root", selfPosted, 0, "arbitrary-id"},
	} {
		chain, ambiguous := ChainToRoot(view, tc.op)
		if ambiguous != 0 {
			t.Errorf("%s: ambiguous = %d, want 0", tc.name, ambiguous)
		}
		if got := ProvableHops(view, tc.op); got != tc.hops {
			t.Errorf("%s: hops = %d, want %d", tc.name, got, tc.hops)
		}
		if got := chain[len(chain)-1].OpID; got != tc.rootOpID {
			t.Errorf("%s: root = %q, want %q", tc.name, got, tc.rootOpID)
		}
	}
}

// A trigger absent from the view (rolled up) still walks from its id and
// author alone — and reads as a root when its parent is gone too.
func TestWalkerHandlesRolledUpTrigger(t *testing.T) {
	m0 := Turn{OpID: "m0", Author: "owner", Type: "turn.post"}
	view := []Turn{m0}
	absent := Turn{OpID: WakeOpID("m0", "bot"), Author: "bot"} // not in view
	if got := ProvableHops(view, absent); got != 1 {
		t.Fatalf("absent trigger hops = %d, want 1 (parent still in view)", got)
	}
	orphan := Turn{OpID: WakeOpID("gone", "bot"), Author: "bot"}
	if got := ProvableHops(view, orphan); got != 0 {
		t.Fatalf("orphan trigger hops = %d, want 0 (a root)", got)
	}
}

// Ambiguity — more than one parent candidate — is reported, never
// absorbed. Constructed via duplicate op ids; a real UUIDv5 collision is
// practically impossible (0 in 421 resolved chains, episode 0128), and
// that is exactly why a silent pick would be corruption, not convenience.
func TestWalkerReportsAmbiguity(t *testing.T) {
	dup1 := Turn{OpID: "m1", Author: "owner", Type: "turn.post", Body: "one"}
	dup2 := Turn{OpID: "m1", Author: "owner", Type: "turn.post", Body: "two"}
	out := Turn{OpID: WakeOpID("m1", "bot"), Author: "bot", Type: "turn.post"}
	_, n := ParentOf([]Turn{dup1, dup2, out}, out)
	if n != 2 {
		t.Fatalf("parent matches = %d, want 2 reported", n)
	}
	_, ambiguous := ChainToRoot([]Turn{dup1, dup2, out}, out)
	if ambiguous != 1 {
		t.Fatalf("ambiguous links = %d, want 1", ambiguous)
	}
}

// The budget decision: the depth bound refuses at the boundary, the window
// floor counts only the persona's own recent turns, zero parts never
// refuse.
func TestBudgetDecision(t *testing.T) {
	now := time.Now()
	root := Turn{OpID: "m1", Author: "owner", Type: "turn.post", Timestamp: now.Add(-time.Minute)}
	a1 := Turn{OpID: WakeOpID("m1", "agent-a"), Author: "agent-a", Type: "turn.post", Timestamp: now.Add(-50 * time.Second)}
	b1 := Turn{OpID: WakeOpID(a1.OpID, "agent-b"), Author: "agent-b", Type: "turn.post", Timestamp: now.Add(-40 * time.Second)}
	old := Turn{OpID: "old", Author: "agent-b", Type: "turn.post", Timestamp: now.Add(-time.Hour)}
	comment := Turn{OpID: "c1", Author: "agent-b", Type: "comment.add", Timestamp: now}
	view := []Turn{root, a1, b1, old, comment}

	for _, tc := range []struct {
		name    string
		b       Budget
		trigger Turn
		persona string
		refuse  bool
	}{
		{"zero budget admits everything", Budget{}, b1, "agent-a", false},
		{"depth admits under the bound", Budget{MaxHops: 3}, b1, "agent-a", false},
		{"depth refuses at the boundary", Budget{MaxHops: 2}, b1, "agent-a", true},
		{"root trigger is hop one", Budget{MaxHops: 1}, root, "agent-a", false},
		{"window refuses at the count", Budget{WindowMax: 1, WindowPer: 10 * time.Minute}, root, "agent-b", true},
		{"window ignores old turns", Budget{WindowMax: 2, WindowPer: 10 * time.Minute}, root, "agent-b", false},
		{"window ignores other authors", Budget{WindowMax: 1, WindowPer: 10 * time.Minute}, root, "agent-c", false},
		{"window counts all own turns inside it", Budget{WindowMax: 2, WindowPer: 2 * time.Hour}, root, "agent-b", true}, // b1 + old
		{"window ignores comments", Budget{WindowMax: 3, WindowPer: 2 * time.Hour}, root, "agent-b", false},              // comment.add would make 3
	} {
		reason, refuse := BudgetDecision(tc.b, view, tc.trigger, tc.persona, now)
		if refuse != tc.refuse {
			t.Errorf("%s: refuse = %v (reason %q), want %v", tc.name, refuse, reason, tc.refuse)
		}
		if refuse && reason == "" {
			t.Errorf("%s: refusal with an empty reason", tc.name)
		}
	}
}

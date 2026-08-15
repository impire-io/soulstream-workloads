package waker

import "testing"

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

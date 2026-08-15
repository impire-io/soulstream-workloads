package wrap

// This file is the pure half (the lifecycle.go pattern): wake identity and
// outcome correlation, no I/O. The package doc lives in wrap.go.

import "github.com/google/uuid"

// wakeNamespace is the fixed UUIDv5 namespace for wake outcome ids. Never
// change it: the derived ids are the duplicate-detection identities of
// outcome ops already in records.
var wakeNamespace = uuid.MustParse("6f1c8f5a-9e0d-4b1c-8a52-7c0b7a0e4d21")

// WakeOpID derives the one outcome-op id a wake may publish under: a UUIDv5
// of the notify op id AND the agent persona, deterministic across
// redeliveries, shared by reply and failure — a wake has one outcome slot,
// whichever kind fills it. The persona is part of the identity because one
// mention can tap several registered agents: a wake is one delivery to one
// agent, and hashing the notify op alone made two agents' outcomes dedupe
// into a single turn (measured by the multi-agent gate test). Staying in
// UUID shape keeps every reader's id assumptions intact, and the id doubles
// as Nats-Msg-Id so the stream's duplicate window absorbs same-wake reposts.
func WakeOpID(notifyOpID, persona string) string {
	return uuid.NewSHA1(wakeNamespace, []byte(notifyOpID+"/"+persona)).String()
}

// Turn is the slice of a topic contribution correlation needs. The narrow
// local type keeps this file pure and the fakes trivial.
type Turn struct {
	OpID   string
	Author string
	Type   string
	Body   string
}

// PostedDuringRun reports a turn the persona posted between the two
// snapshots — i.e. during this harness run. Correlation MUST compare
// snapshots, never anchor on stream order: with several mentions in one
// topic, an earlier wake's reply outranks a later mention's anchor and
// masquerades as its answer (the measured trap, research episode 0082).
func PostedDuringRun(before, after []Turn, persona string) (string, bool) {
	seen := make(map[string]bool, len(before))
	for _, t := range before {
		seen[t.OpID] = true
	}
	for _, t := range after {
		if t.Author == persona && t.Type == "turn.post" && !seen[t.OpID] {
			return t.OpID, true
		}
	}
	return "", false
}

// ContainsOp reports whether the view already holds the given op — the
// redelivery pre-check: a wake whose outcome already landed is acked, not
// re-run, at any redelivery distance (beyond the stream's duplicate window).
func ContainsOp(view []Turn, opID string) bool {
	for _, t := range view {
		if t.OpID == opID {
			return true
		}
	}
	return false
}

// AnchorBody returns the body of the anchoring op when the view still holds
// it ("" after a rollup) — the mention text the prompt carries.
func AnchorBody(view []Turn, opID string) string {
	for _, t := range view {
		if t.OpID == opID {
			return t.Body
		}
	}
	return ""
}

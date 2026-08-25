package wrap

// This file is the pure half (the lifecycle.go pattern): wake identity,
// outcome correlation, the ancestry walker, and the wake-budget decision —
// no I/O. The package doc lives in wrap.go.

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

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
// local type keeps this file pure and the fakes trivial. Timestamp is the
// contribution's server-stamped stream time — the window budget's clock.
type Turn struct {
	OpID      string
	Author    string
	Type      string
	Body      string
	Timestamp time.Time
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

// ParentOf returns the op that provably triggered op — the candidate whose
// WakeOpID binding with op's author yields op's id — and how many candidates
// matched. Zero means op is a chain root (a human post, a rolled-up
// ancestor, or an outcome posted under an arbitrary id); more than one is
// reported ambiguity, never silently resolved (research episode 0128: zero
// ambiguous matches across 421 resolved chains, but a silent wrong pick
// would corrupt both the budget and the diagnostic).
func ParentOf(view []Turn, op Turn) (Turn, int) {
	var match Turn
	n := 0
	for _, cand := range view {
		if cand.OpID == op.OpID {
			continue
		}
		if WakeOpID(cand.OpID, op.Author) == op.OpID {
			match = cand
			n++
		}
	}
	return match, n
}

// ChainToRoot walks provable parents from op upward: chain[0] is op, the
// last element is the chain's root. The walk binds by outcome id, never
// stream order (the 0082 correlation lesson). ambiguous counts links where
// more than one parent matched.
func ChainToRoot(view []Turn, op Turn) (chain []Turn, ambiguous int) {
	chain = []Turn{op}
	cur := op
	for range len(view) { // bounded: a chain cannot outgrow the view
		p, n := ParentOf(view, cur)
		if n == 0 {
			return chain, ambiguous
		}
		if n > 1 {
			ambiguous++
		}
		chain = append(chain, p)
		cur = p
	}
	return chain, ambiguous
}

// ProvableHops is op's provable distance from a chain root — the unit the
// depth budget counts.
func ProvableHops(view []Turn, op Turn) int {
	chain, _ := ChainToRoot(view, op)
	return len(chain) - 1
}

// BudgetDecision reports whether a wake for persona, triggered by trigger,
// must be refused under b — with a legible reason carrying the numbers. A
// zero part never refuses; a wholly zero Budget admits everything. The
// trigger needs only its op id and author, so a trigger absent from the
// view (rolled up) still walks — and reads as a root when its own parent
// is gone too.
func BudgetDecision(b Budget, view []Turn, trigger Turn, persona string, now time.Time) (reason string, refuse bool) {
	if b.MaxHops > 0 {
		if hops := ProvableHops(view, trigger); hops+1 > b.MaxHops {
			return fmt.Sprintf("depth budget: outcome would sit %d provable hops from the root, bound %d", hops+1, b.MaxHops), true
		}
	}
	if b.WindowMax > 0 {
		cut := now.Add(-b.WindowPer)
		n := 0
		for _, t := range view {
			if t.Author == persona && t.Type == "turn.post" && t.Timestamp.After(cut) {
				n++
			}
		}
		if n >= b.WindowMax {
			return fmt.Sprintf("window budget: %d own turns within %s, bound %d", n, b.WindowPer, b.WindowMax), true
		}
	}
	return "", false
}

# Episode 0011 — Pinned to the record: the soulstream replace drops (2026-08-01)

A small change with a consumability meaning: soulrealm's `go.mod` carried
`replace github.com/impire-io/soulstream => ../soulstream` since M1.1 —
correct while the two repos co-evolved daily, but it made soulrealm
unpinnable for any consumer (soulnode's `single-binary-composition`
research named it a release blocker: a module with a filesystem replace on
main cannot be required as a tagged dependency [measured, its
embed-surfaces reading]). The operator's 2026-08-01 direction — expose the
right constructs downstream — covers release shape as much as package
shape.

Measured before landing: soulstream's main is `v0.6.0` plus four docs-only
commits, so the tag is current code-wise [measured, `git describe` +
`git log v0.6.0..main`]; with the replace dropped and
`github.com/impire-io/soulstream v0.6.0` pinned, the full gate runs green —
`make fmt && make test && make lint`, including the integration suite
against the in-process operator-mode server [measured].

What changes day to day: co-developing against an unreleased soulstream now
uses an untracked local `go.work` (or a temporary replace that never lands
on main), and soulstream API changes reach soulrealm by tag bump — the same
discipline soulidentity's `e2e/` module already lives by (it pins soulstream
v0.6.0 with no replace).

Refuted/reversed: nothing — the replace was right for its era; the era
ended when a consumer needed to pin.

Reversal condition: a period of genuinely lockstep co-development where
tag-bumping measurably stalls the work (observable: soulstream tags cut
solely to unblock soulrealm builds, more than once a week) reopens the
replace — as a stated temporary measure with its removal condition attached.

Trail: `go.mod`/`go.sum`; soulnode `hq/01-RESEARCH/single-binary-composition/embed-surfaces.md`;
soulidentity `e2e/go.mod` (the prior art). Commit: this change.

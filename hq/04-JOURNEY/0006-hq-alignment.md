# Episode 0006 — HQ alignment: the lint gets built (2026-07-24)

An audit found the hq coherent but with four gaps between what the documents
promised and what the repo held. This episode closes them.

**The structural lint now exists** [measured]. The constitution (Article VI)
and the journey-log / research-graduate skills had cited an "hq structural
lint" as the enforcement backbone — but it was never built; the phrasing was
all future tense ("once it exists", "once written"). It is now
`internal/hqlint`, a test-only Go package that runs under `go test ./...`, so
`make test` and the commit gate catch hq/ drift. Ported from pra's
`tests/test_hq_structure.py` and held to soulrealm's own rules: the five areas
plus their READMEs, the four GENESIS files, and both TEMPLATEs exist; research
topics carry a legal non-terminal state; journey episodes match `NNNN-slug.md`,
are unique, contiguous from 0001, indexed — and, unlike pra which exempts its
pre-split episodes, **every** episode carries a `Reversal condition:` line; the
`.specify/memory/constitution.md` symlink resolves into GENESIS; and relative
markdown links inside hq/ resolve. Verified it fails on a planted violation (a
non-contiguous, unindexed, reversal-less episode with a broken link), then goes
green when the plant is removed.

**Stale statuses fixed** [measured]. README.md and CLAUDE.md still said "no code
yet"; code had landed the same day (episodes 0004/0005, the roadmap, and
Where-things-stand were already correct). Both now read: Phase 1 in progress,
M1.1 and M1.2 done, M1.3 next.

**Specs marked Shipped, with an honest deviation recorded** [judgment]. Specs
001 and 002 both still read `Status: Draft` though both shipped; set to
`Shipped`. Spec 002 lacked tasks.md and its supporting artifacts because the
spec-kit flow was short-circuited for M1.2 (it shipped from spec.md + plan.md
only). Those artifacts are **not** reconstructed after the fact — a process
note in 002's spec.md records the short-circuit instead.

**The full spec-kit flow is vendored** [measured]. `.specify` had been a
minimal setup; with the owner's approval it now mirrors pra: the 14 speckit
skills, the bash workflow scripts, the git branching extension and its hook
registry, the SDD workflow, and the install manifests. Vendoring surfaced a
latent bug — soulrealm's `plan-template.md` and `tasks-template.md` had baked
in **pra's** constitution principles (NATS-Native First, Smallest Viable
Implementation, ELI5 Documentation), none of which are soulrealm's, so a
`/speckit-plan` here would have Constitution-Checked against the wrong articles.
Replaced with pra's generic versions that keep the "[Gates determined based on
constitution file]" placeholder, so the check reads soulrealm's own
constitution through the symlink.

**Constitution amended 0.1.0 → 0.1.1** (PATCH, clarification): Article VI drops
the "once it exists" qualifier now that the lint does. No spec-kit template
embeds Article VI text — the Constitution Check reads it live via the symlink —
so no template propagation was required. This is a clarification, not a
direction change, so the working agreement's adversarial pass does not bind; it
is recorded here per the governance amendment rule.

Nothing was refuted or reversed — the audit's four gaps were real and are
closed as found.

Reversal condition: if `internal/hqlint` is ever dropped from `go test ./...`,
Article VI's parenthetical must return to naming the lint as future work and
this episode's "the lint now exists" claim no longer holds.

Trail: the lint in `internal/hqlint` (doc.go + hqlint_test.go); amended
[constitution.md](../00-GENESIS/constitution.md) Article VI; softened the
journey-log and research-graduate skills; specs
[001](../../specs/001-launch-an-agent/spec.md) and
[002](../../specs/002-call-a-tool/spec.md) marked Shipped; vendored spec-kit
under [`.specify`](../../.specify/README.md); [roadmap](../03-IMPLEMENTATION/roadmap.md)
refreshed. Commits 73aeee5, 5431fa2, d5d67d0, f30e0ed, and this one.

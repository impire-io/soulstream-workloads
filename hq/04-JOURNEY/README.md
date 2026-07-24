# 04-JOURNEY — the narrative record

What was built, what was measured, what was believed and then refuted, and
what each episode taught. Specs say what the system *is*; these episodes say
how we *got here* — including the dead ends, because the refuted hypotheses
are as load-bearing as the shipped code.

> **Keeping this log alive:** whenever a feature lands, a research
> investigation concludes, or a load-bearing decision is made, add a numbered
> episode with `/journey-log` (research topics get theirs via
> `/research-graduate`). Follow [`TEMPLATE.md`](TEMPLATE.md) — including its
> required Reversal-condition line and evidence-class tags. Honesty rules
> apply here as everywhere: record what actually happened, including failures,
> reversals, and findings that contradicted expectations. This duty is
> anchored in `../00-GENESIS/how-we-work.md`.

## Where things stand (2026-07-24)

**The project was founded** ([episode 0001](0001-genesis.md)): soulrealm is the
runtime companion to soulstream — soulstream records, soulrealm runs. The hq is
bootstrapped from the sibling project's proven structure, with a constitution
whose non-negotiable article is the **substrate boundary** (soulrealm never
becomes a store of record; everything worth keeping flows back to topics as
ops).

**The substrate question is decided** ([episode
0002](0002-the-substrate-decision.md)): after a live NEX spike and a source
read, **soulrealm builds its own runtime — NEX as influence, not dependency —
with the soulstream op-log as the single control plane.** Measured: role
(`agent`/`tool`) is orthogonal to lifecycle (`service`/`function`/`job`); NEX
issues scoped per-workload identity for free and is embeddable via public
options (so a fork was never forced). The `[judgment]` call to rebuild rather
than embed turned on **not running a second control plane** (`$NEX.control.*`)
beside the op-log — recorded after teach-back, with the embed case argued at
full strength first. That opened design doc
[`0001-soulrealm-runtime.md`](../02-DESIGN/0001-soulrealm-runtime.md): the
single-plane runtime, the role×lifecycle model, a realm-semantic per-workload
minter, lifecycle-as-ops, and pluggable backends, with an honest NEX influence
ledger and its `[O]` open sub-questions.

**The first slice is specced** ([`specs/001-launch-an-agent/spec.md`](../../specs/001-launch-an-agent/spec.md)):
declare an `agent`/`service`, mint a persona-scoped credential, launch it
native, post a turn as its persona, lifecycle visible as ops — no second
control plane. Minimal spec-kit scaffolding is in place (`.specify/` with the
constitution symlinked). The signing story is resolved soulrealm-held.

**Scope is soulstream-only** ([episode 0003](0003-soulstream-only-scope.md)):
soulrealm depends on soulstream and nothing else — no Impire-platform services
(identity, tenancy, vault) for now. The minter stays a seam for a future
external authority, but none is designed in.

**M1.1 is implemented** ([episode 0004](0004-the-first-agent-runs.md)): the Go
module `github.com/impire-io/soulrealm` exists, and an agent launched by
soulrealm posts a turn attributed to its persona while its lifecycle shows up
as `work.open/claim/done` on the topic — proven end-to-end (SC-001, SC-002),
whole gate green. The plan's bet held: **no new soulstream vocabulary** —
soulrealm is the work.md "runner". Six packages (declaration, minter,
backend/native, runner, two cmds), pure logic split from I/O so most tests need
no server; the native backend proves it does not leak soulrealm's secrets into
a workload. All five success criteria met (SC-003 enforcement via an in-process
operator-mode server).

**M1.2 is done** ([episode 0005](0005-a-tool-answers.md)): soulrealm launches a
`tool` service and an agent discovers it by name and calls it (uppercase round
trip). Added the tool role, role-aware scopes, and the runner's launch/stop
(services don't self-exit). A measured lesson landed a boundary: tool
request-reply is transient, so it rides soulrealm's own `SOULREALM.SVC.*`
subjects, not the stored `SOULSTREAM.>` stream (which would ack and race the
reply). SC-001/002/003 proven end-to-end.

**The hq is now aligned with its own contract** ([episode
0006](0006-hq-alignment.md)): the "hq structural lint" the constitution and
skills had cited as the enforcement backbone is finally built —
`internal/hqlint`, a test-only Go package that rides `make test` and the commit
gate. Along the way: README/CLAUDE status corrected to Phase 1 (M1.1 + M1.2
done); specs 001/002 marked Shipped with 002's spec-kit short-circuit recorded
honestly; the full spec-kit flow vendored from pra (which also fixed a
plan/tasks template that had baked in pra's constitution principles); and
Article VI clarified (constitution 0.1.1). **Next:** M1.3 (the same
declarations under a second isolation backend — Docker/Firecracker — proving
constitution III's backend-orthogonality).

## Episode index

| # | Episode |
|---|---|
| 0001 | [Genesis: soulrealm gets an HQ](0001-genesis.md) |
| 0002 | [The substrate decision: a from-scratch, NEX-influenced runtime](0002-the-substrate-decision.md) |
| 0003 | [Soulstream-only scope: the platform waits](0003-soulstream-only-scope.md) |
| 0004 | [The first agent runs](0004-the-first-agent-runs.md) |
| 0005 | [A tool answers](0005-a-tool-answers.md) |
| 0006 | [HQ alignment: the lint gets built](0006-hq-alignment.md) |

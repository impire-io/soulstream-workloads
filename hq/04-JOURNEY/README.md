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

## Where things stand (2026-07-22)

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

**The M1.1 plan is drafted** (`specs/001-launch-an-agent/` plan + research +
data-model + contracts + quickstart). Headline finding: the slice needs **no
new soulstream vocabulary** — soulrealm is the "runner" persona work.md stage 4
already anticipates, and lifecycle maps onto the existing
`work.open/claim/done/abandon` ops. Two peer identities (runner + workload), a
realm-semantic minter, native-backend `os/exec`, and (refining design 0001 §4)
direct env cred-injection for local single-node exec (xkey delivery is a
multi-node concern). **Next:** `/speckit-tasks` → implementation (creates the Go
module).

## Episode index

| # | Episode |
|---|---|
| 0001 | [Genesis: soulrealm gets an HQ](0001-genesis.md) |
| 0002 | [The substrate decision: a from-scratch, NEX-influenced runtime](0002-the-substrate-decision.md) |
| 0003 | [Soulstream-only scope: the platform waits](0003-soulstream-only-scope.md) |

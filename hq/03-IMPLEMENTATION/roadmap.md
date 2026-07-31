# Roadmap

The live plan. No dates — **gates, not calendars**. Every milestone names the
research gate it depends on; nothing is built ahead of its gate (constitution
IV).

## Where we are

**Phase 1 complete — M1.1, M1.2, and M1.3 have all landed.** The hq is
bootstrapped ([episode 0001](../04-JOURNEY/0001-genesis.md)) and the substrate
question is closed ([episode
0002](../04-JOURNEY/0002-the-substrate-decision.md)): a from-scratch,
NEX-influenced runtime with the op-log as the single control plane, specified
in design [`0001-soulrealm-runtime.md`](../02-DESIGN/0001-soulrealm-runtime.md).
An agent runs and a tool answers (episodes 0004/0005); the hq's own structural
lint rides the gate ([episode 0006](../04-JOURNEY/0006-hq-alignment.md)); and
the same declarations now run unchanged inside microsandbox microVMs —
constitution III proven by a second backend ([episode
0007](../04-JOURNEY/0007-a-second-wall.md)). **Phase 2 is complete —
M2.1 landed** ([episode 0008](../04-JOURNEY/0008-kubernetes-backend.md)
research → [episode 0009](../04-JOURNEY/0009-a-third-wall-lands.md) build):
the same declarations run as Kubernetes pods, artifact via a per-run OCI
image through the operator's registry, credential as a Secret. **The Fleet
research gate is met** ([episode 0010](../04-JOURNEY/0010-fleet.md), all
three pre-registered bars measured PASS) — Phase 3 is unblocked with design
[`0003-fleet.md`](../02-DESIGN/0003-fleet.md); the remaining horizons
(sandboxes-stage-5, the tool ecosystem) stay gated — see below.

## Phase 0 — Substrate (research) — ✅ closed 2026-07-22

**Gate met.** `nex-runtime-substrate` graduated to design (episode 0002).
Decided: NEX is influence, not dependency; `agent`/`tool` is a role axis
orthogonal to the `service`/`function`/`job` lifecycle axis; the backend seam
is soulrealm-owned (constitution III), emitting ops rather than a second
control plane.

## Phase 1 — First workload (design → build) — *unblocked*

Runs the spec-kit flow against design 0001 (§9 acceptance criteria). Exit
criteria, made precise per feature in `specs/NNN-*/`:

- **M1.1 — Launch one agent.** ✅ **Done** ([episode
  0004](../04-JOURNEY/0004-the-first-agent-runs.md); spec/plan/tasks in
  [`specs/001-launch-an-agent/`](../../specs/001-launch-an-agent/)). The Go
  module exists; an agent launches natively, posts a turn attributed to its
  persona, and its lifecycle is `work.open/claim/done` on the topic — proven
  end-to-end (SC-001, SC-002). The plan's bet held: **no new soulstream
  vocabulary** (soulrealm is the work.md "runner"). Signing is soulrealm-held
  (episode 0003). SC-003 (scope *enforcement*) is now proven against an
  operator-mode server; SC-004/SC-005 at unit level. Whole gate green, all five
  success criteria met.
- **M1.2 — Launch one tool, called by the agent.** ✅ **Done** ([episode
  0005](../04-JOURNEY/0005-a-tool-answers.md); [`specs/002-call-a-tool/`](../../specs/002-call-a-tool/)).
  A tool workload the agent discovers by name and calls over request-reply
  (uppercase round trip), under the same one-identity model. Added the `tool`
  role, role-aware scopes, and the runner's launch/stop (services don't
  self-exit). Measured lesson: tool RPC is transient, so it rides soulrealm's
  own `SOULREALM.SVC.*` (the `SOULSTREAM.>` stream would otherwise ack and race
  it). SC-001/002/003 proven end-to-end; gate green.
- **M1.3 — Second backend.** ✅ **Done** ([episode
  0007](../04-JOURNEY/0007-a-second-wall.md);
  [`specs/003-microsandbox-backend/`](../../specs/003-microsandbox-backend/)).
  **Open amendment:** the backend landed as **microsandbox** (microVM via
  libkrun), not the "Docker or Firecracker" written here at planning time —
  microVM-grade isolation that also runs on the macOS dev machine (Firecracker
  cannot; Docker-on-mac is one shared daemon VM). Measured: the byte-identical
  M1.1/M1.2 declarations (asserted in-test) ran sandboxed — agent turn +
  `open/claim/done`, tool discovery + round trip, crash → `abandon`, an
  isolation probe readable natively but denied in-guest, zero sandboxes left
  after every end-of-life. The seam held: runner, minter, declaration all
  untouched; the `msb` CLI is supervised as a child process (no CGO SDK);
  loopback NATS is rewritten to the guest's host alias under a host-only
  network policy. Gate: `make check && make test-msb` green. Named
  limitation: a non-loopback NATS server needs the `public` net profile
  (Fleet-era). Upstream bug found and worked around: msb 0.6.7 cannot mount
  symlink-traversing sources.

## Phase 2 — The Kubernetes backend (design → build) — ✅ complete

**Gate met 2026-07-29** (research [episode
0008](../04-JOURNEY/0008-kubernetes-backend.md), all four pre-registered
bars measured PASS); **landed the same day**.

- **M2.1 — Kubernetes backend.** ✅ **Done** ([episode
  0009](../04-JOURNEY/0009-a-third-wall-lands.md);
  [`specs/004-kubernetes-backend/`](../../specs/004-kubernetes-backend/)).
  All exit criteria met, measured on a real kind cluster + local OCI
  registry: the byte-identical M1.1/M1.2 declarations run as pods with the
  identical op mapping (native control arm asserting the declaration
  byte-for-byte); crash → `work.abandon`; an out-of-band pod deletion still
  closes as `work.abandon` with no resurrected copy; the scope probe inside
  a pod against an operator-mode NATS is denied out-of-scope with its
  credential Secret-delivered (and never on host disk); zero pods/Secrets
  after every end of life; runner, minter, and declaration untouched
  (`backend/natsurl` extracted below the seam, msb suite still green). The
  two `[O]`s were decided in the plan: **an OCI-registry artifact channel**
  (a recorded reversal — the plan's HTTP draft was rejected by the
  maintainer; an open amendment to design 0002's candidates, propagated) and
  **client-go inside `backend/k8s`** (after teach-back). Gate:
  `make check && make test-k8s` green (five e2e scenarios, ~26 s;
  environment via `scripts/kind-registry.sh up`).

## Phase 3 — Fleet (design → build) — *unblocked*

**Gate met 2026-07-31** (research [episode
0010](../04-JOURNEY/0010-fleet.md): three pre-registered bars measured PASS
across four spikes; two open reversals on the record). Decided: placement
**is** `work.claim` (no auction, no coordinator); reclaim is *projection
nominates → probe vetoes → ordinary `work.abandon` decides*; nodes are
homogeneous with the minter role dissolved into the identity plane
(`soulidentity`). Design: [`0003-fleet.md`](../02-DESIGN/0003-fleet.md).

- **M3.1 — first fleet milestone.** Runs the spec-kit flow against design
  0003 (§8 acceptance criteria: two real nodes, contested placement,
  kill → reclaim within bound, seedless scoped launch, seams untouched).
  Exit criteria made precise per feature in `specs/NNN-*/`. External
  dependency, tracked openly: the preferred minting path needs
  soulidentity to stamp tags on mints (its M2 "consumer-proven" clause);
  the measured delegated-minting fallback works today.

## Later horizons (named, not planned)

Each will get its own research gate when it approaches:

- **Sandboxes.** Soulstream work-extension stage 5 — the physical bench —
  gated on stage-4 execution being real in soulstream.
- **Tool ecosystem.** MCP servers and exec sandboxes as first-class,
  discoverable realm tools.

## Discipline

Exit criteria are written before the work and amended only openly with the raw
findings recorded. Landing a feature updates this file, writes a journey
episode, and propagates design changes — in the same merge (constitution VI).

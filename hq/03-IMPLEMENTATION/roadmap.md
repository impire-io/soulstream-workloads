# Roadmap

The live plan. No dates — **gates, not calendars**. Every milestone names the
research gate it depends on; nothing is built ahead of its gate (constitution
IV).

## Where we are

**Substrate decided; design open.** The hq is bootstrapped ([episode
0001](../04-JOURNEY/0001-genesis.md)) and the substrate question is closed
([episode 0002](../04-JOURNEY/0002-the-substrate-decision.md)): a from-scratch,
NEX-influenced runtime with the op-log as the single control plane, specified in
design [`0001-soulrealm-runtime.md`](../02-DESIGN/0001-soulrealm-runtime.md).
No code yet. Phase 0 is closed; Phase 1 is now unblocked.

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
- **M1.3 — Second backend.** ⬜ Next. The same agent + tool declarations run
  unchanged under a second backend (Docker or Firecracker), proving constitution
  III. The `Backend` interface + declaration are already backend-agnostic (M1.1
  SC-005); this milestone adds the second implementation and proves the
  declaration is untouched.

## Later horizons (named, not planned)

Held behind Phase 1; each will get its own research gate when it approaches:

- **Fleet.** More than one node; location-transparent scheduling.
- **Sandboxes.** Soulstream work-extension stage 5 — the physical bench —
  gated on stage-4 execution being real in soulstream.
- **Tool ecosystem.** MCP servers and exec sandboxes as first-class,
  discoverable realm tools.

## Discipline

Exit criteria are written before the work and amended only openly with the raw
findings recorded. Landing a feature updates this file, writes a journey
episode, and propagates design changes — in the same merge (constitution VI).

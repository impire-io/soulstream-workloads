# Roadmap

The live plan. No dates — **gates, not calendars**. Every milestone names the
research gate it depends on; nothing is built ahead of its gate (constitution
IV).

## Where we are

**Genesis.** The hq is bootstrapped ([episode 0001](../04-JOURNEY/0001-genesis.md)).
No design has graduated; no code exists. The one open gate is the substrate
question.

## Phase 0 — Substrate (research)

**Gate:** [`01-RESEARCH/nex-runtime-substrate`](../01-RESEARCH/nex-runtime-substrate/README.md)
graduates. Decides: NEX yes/no; the `agent`/`tool` modelling axis and the
layer it lives at; the backend seam.

Nothing in later phases is designed until this closes.

## Phase 1 — First workload (design → build) — *gated on Phase 0*

Exit-criteria sketch, to be made precise when Phase 0 graduates into a design
doc:

- **M1.1 — Launch one agent.** A single long-lived agent persona launched onto
  a real NEX node under the decided contract, receiving scoped credentials,
  participating in a soulstream topic (posts a turn), lifecycle visible as ops
  (constitution V). One isolation backend (native process) only.
- **M1.2 — Launch one tool, called by the agent.** A tool workload the agent
  discovers and calls over the realm transport, under the same identity model.
- **M1.3 — Second backend.** The same two workload declarations run unchanged
  under a second backend (Docker or Firecracker), proving constitution III.

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

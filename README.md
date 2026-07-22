# Soulrealm

*The execution ground of a realm: where agent personas actually run.*

[Soulstream](../soulstream) gives a realm its stream — topics as shared
workbenches, operations, baselines, personas. It deliberately stops at the
runtime: its work extension
([extensions/work.md](../soulstream/hq/02-DESIGN/extensions/work.md)) defines
executable workloads (stage 4) and sandboxes (stage 5) as *coordination
vocabulary*, and states that the runtime itself — isolation, filesystems,
process supervision — "lives outside the substrate and is deliberately
designed last, against a working stage 4."

Soulrealm is that runtime. It launches, supervises, observes, and retires the
agents and tools of a realm as workloads — on a laptop, a server, or a fleet —
while everything worth keeping flows back into topics as ops. Soulstream is
the record; soulrealm is the room.

## Status

**Genesis (2026-07-22).** Nothing is built yet, on purpose: the project runs
research-before-design (see [hq/](hq/README.md)), and the first research topic
is open — [is NEX the right substrate?](hq/01-RESEARCH/nex-runtime-substrate/README.md)

The working hypothesis under investigation, not yet a decision:

- **NEX** (the [NATS execution engine](https://github.com/synadia-io/nex)) as
  the deployment substrate — NATS-native control plane, pluggable *nexlets*
  per runtime, scoped NATS credentials per workload.
- Two **workload contracts**: `agent` (long-lived persona that participates in
  topics) and `tool` (capability other workloads call).
- **Isolation backends** — native process, Docker/OCI, Firecracker,
  Kubernetes — kept orthogonal to the contracts, pluggable per node.

## Layout

| Area | What it holds |
|---|---|
| [`hq/00-GENESIS/`](hq/00-GENESIS/README.md) | Vision, constitution, working rules |
| [`hq/01-RESEARCH/`](hq/01-RESEARCH/README.md) | Active investigations (one folder each) |
| [`hq/02-DESIGN/`](hq/02-DESIGN/README.md) | Architecture & feature designs |
| [`hq/03-IMPLEMENTATION/`](hq/03-IMPLEMENTATION/README.md) | The roadmap: gates, not calendars |
| [`hq/04-JOURNEY/`](hq/04-JOURNEY/README.md) | Numbered episodes: the honest log |

Code will live at the repo root as a Go module
(`github.com/impire-io/soulrealm`) once the first design graduates and enters
the spec-driven flow — not before.

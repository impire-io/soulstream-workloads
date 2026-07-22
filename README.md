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

**Substrate decided (2026-07-22); design open, no code yet.** The project runs
research-before-design (see [hq/](hq/README.md)). The opening question — what
runtime substrate — is settled ([journey
0002](hq/04-JOURNEY/0002-the-substrate-decision.md)): after a live
[NEX](https://github.com/synadia-io/nex) spike and source read, **soulrealm
builds its own runtime — NEX as influence, not dependency — with the soulstream
op-log as the single control plane.** The architecture is in design doc
[`0001-soulrealm-runtime.md`](hq/02-DESIGN/0001-soulrealm-runtime.md).

The decided shape:

- **One control plane.** A workload's whole visible life is operations on
  topics; there is no second coordination system beside the op-log.
- **Two orthogonal axes.** *Role* — `agent` (long-lived persona that
  participates in topics) and `tool` (capability other workloads call) — is
  independent of *lifecycle* — `service` / `function` / `job` (borrowed from
  NEX).
- **Realm-semantic identity.** Each workload gets a freshly minted NATS user
  scoped to its persona's subjects, delivered via an xkey-encrypted env
  (NEX's `CredVendor` design, reimplemented).
- **Isolation backends** — native process, Docker/OCI, Firecracker, Kubernetes
  — kept orthogonal to the contract, pluggable per node.

Next: the first runtime slice through the spec-kit flow (roadmap Phase 1).

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

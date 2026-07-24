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

**Phase 1 in progress — M1.1 and M1.2 have landed; M1.3 (a second isolation
backend) is next.** The opening substrate question is settled ([journey
0002](hq/04-JOURNEY/0002-the-substrate-decision.md)): after a live
[NEX](https://github.com/synadia-io/nex) spike and source read, **soulrealm
builds its own runtime — NEX as influence, not dependency — with the soulstream
op-log as the single control plane.** The architecture is in design doc
[`0001-soulrealm-runtime.md`](hq/02-DESIGN/0001-soulrealm-runtime.md), and the
Go module `github.com/impire-io/soulrealm` now exists and runs.

What already works:

- **M1.1 — an agent runs** ([journey
  0004](hq/04-JOURNEY/0004-the-first-agent-runs.md)): soulrealm launches an
  agent natively with a freshly minted, persona-scoped credential; its posted
  turn is attributed to that persona while its lifecycle appears as
  `work.open/claim/done` on the topic — no second control plane.
- **M1.2 — a tool answers** ([journey
  0005](hq/04-JOURNEY/0005-a-tool-answers.md)): an agent discovers a launched
  tool by name and calls it over request-reply (uppercase round trip), under
  the same one-identity model.

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

Next: M1.3 — the same agent and tool declarations run unchanged under a second
isolation backend (Docker or Firecracker), proving constitution III (roadmap
Phase 1).

## Layout

| Area | What it holds |
|---|---|
| [`hq/00-GENESIS/`](hq/00-GENESIS/README.md) | Vision, constitution, working rules |
| [`hq/01-RESEARCH/`](hq/01-RESEARCH/README.md) | Active investigations (one folder each) |
| [`hq/02-DESIGN/`](hq/02-DESIGN/README.md) | Architecture & feature designs |
| [`hq/03-IMPLEMENTATION/`](hq/03-IMPLEMENTATION/README.md) | The roadmap: gates, not calendars |
| [`hq/04-JOURNEY/`](hq/04-JOURNEY/README.md) | Numbered episodes: the honest log |

Code lives at the repo root as the Go module `github.com/impire-io/soulrealm`;
each feature's spec-kit artifacts freeze under `specs/NNN-*/` as it lands.

## License

Soulrealm is released under the [MIT License](LICENSE) — © 2026 Daan Gerits.

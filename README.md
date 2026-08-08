# Soulrealm

*The execution ground of a realm: where agent personas actually run.*

[Soulstream](../soulstream) gives a realm its stream — topics as shared
workbenches, operations, baselines, personas. It deliberately stops at the
runtime: its work extension
([extensions/work.md](../soulstream/../soul-hq/02-DESIGN/soulrealm/extensions/work.md)) defines
executable workloads (stage 4) and sandboxes (stage 5) as *coordination
vocabulary*, and states that the runtime itself — isolation, filesystems,
process supervision — "lives outside the substrate and is deliberately
designed last, against a working stage 4."

Soulrealm is that runtime. It launches, supervises, observes, and retires the
agents and tools of a realm as workloads — on a laptop, a server, or a fleet —
while everything worth keeping flows back into topics as ops. Soulstream is
the record; soulrealm is the room.

## Status

**Phase 1 complete — M1.1, M1.2, and M1.3 have landed.** The opening
substrate question is settled ([journey
0002](../soul-hq/04-JOURNEY/0002-soulrealm-the-substrate-decision.md)): after a live
[NEX](https://github.com/synadia-io/nex) spike and source read, **soulrealm
builds its own runtime — NEX as influence, not dependency — with the soulstream
op-log as the single control plane.** The architecture is in design doc
[`0001-soulrealm-runtime.md`](../soul-hq/02-DESIGN/soulrealm/0001-soulrealm-runtime.md), and the
Go module `github.com/impire-io/soulrealm` now exists and runs.

What already works:

- **M1.1 — an agent runs** ([journey
  0004](../soul-hq/04-JOURNEY/0004-soulrealm-the-first-agent-runs.md)): soulrealm launches an
  agent natively with a freshly minted, persona-scoped credential; its posted
  turn is attributed to that persona while its lifecycle appears as
  `work.open/claim/done` on the topic — no second control plane.
- **M1.2 — a tool answers** ([journey
  0005](../soul-hq/04-JOURNEY/0005-soulrealm-a-tool-answers.md)): an agent discovers a launched
  tool by name and calls it over request-reply (uppercase round trip), under
  the same one-identity model.
- **M1.3 — a second wall** ([journey
  0007](../soul-hq/04-JOURNEY/0020-soulrealm-a-second-wall.md)): the byte-identical declarations
  run inside [microsandbox](https://github.com/superradcompany/microsandbox)
  microVMs — agent and tool both — proving constitution III with a real
  isolation boundary (a host path readable natively is denied in-guest).
  Backend selection is node-side (`SOULREALM_BACKEND=msb`); the declaration
  cannot even name a backend.

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
- **Isolation backends** — native process and microsandbox microVMs today;
  Docker/OCI, Firecracker, Kubernetes as later options — kept orthogonal to
  the contract, pluggable per node.

Next: the later horizons get their research gates — Fleet (multi-node),
sandboxes (soulstream work stage 5), and the tool ecosystem (roadmap).

## Layout

| Area | What it holds |
|---|---|
| [`../soul-hq/00-GENESIS/`](../soul-hq/00-GENESIS/README.md) | Vision, constitution, working rules |
| [`../soul-hq/01-RESEARCH/`](../soul-hq/01-RESEARCH/README.md) | Active investigations (one folder each) |
| [`../soul-hq/02-DESIGN/soulrealm/`](../soul-hq/02-DESIGN/soulrealm/README.md) | Architecture & feature designs |
| [`../soul-hq/03-IMPLEMENTATION/`](../soul-hq/03-IMPLEMENTATION/README.md) | The roadmap: gates, not calendars |
| [`../soul-hq/04-JOURNEY/`](../soul-hq/04-JOURNEY/README.md) | Numbered episodes: the honest log |

Code lives at the repo root as the Go module `github.com/impire-io/soulrealm`;
each feature's spec-kit artifacts freeze under `specs/NNN-*/` as it lands.

## License

Soulrealm is [fair-code](https://faircode.io) licensed under the
[Sustainable Use License](LICENSE) — © 2026 Daan Gerits. Free to use, modify,
and self-host for internal or non-commercial use; offering it to others as a
paid product or service requires an agreement — see
[impire.io/license](https://impire.io/license/). Versions released before this
change remain MIT.

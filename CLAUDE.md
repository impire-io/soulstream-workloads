# Soulrealm — orientation

**Read [`../soul-hq/`](../soul-hq/README.md) first.** Everything about how this project is run
lives there: the vision and constitution ([`../soul-hq/00-GENESIS/`](../soul-hq/00-GENESIS/README.md)),
active research ([`../soul-hq/01-RESEARCH/`](../soul-hq/01-RESEARCH/README.md)), designs
([`../soul-hq/02-DESIGN/soulstream-workloads/`](../soul-hq/02-DESIGN/soulstream-workloads/README.md)), the roadmap
([`../soul-hq/03-IMPLEMENTATION/`](../soul-hq/03-IMPLEMENTATION/README.md)), and the honest log
([`../soul-hq/04-JOURNEY/`](../soul-hq/04-JOURNEY/README.md)).

## What this is

Soulrealm is the **runtime** of a [soulstream](../soulstream) realm — it
launches, supervises, observes, and retires agents and tools as workloads.
Soulstream is the record; soulstream-workloads is the room.

## Status

**Phase 1 complete — M1.1, M1.2, and M1.3 have landed.** The runtime is a
from-scratch, **NEX-influenced** build with the soulstream op-log as the
*single control plane* — decided in [journey
0002](../soul-hq/04-JOURNEY/0002-soulstream-workloads-the-substrate-decision.md), specified in design
[`0001-soulstream-workloads-runtime.md`](../soul-hq/02-DESIGN/soulstream-workloads/0001-soulstream-workloads-runtime.md). NEX is
influence, not a dependency. The Go module exists and runs: an agent launches
and posts a turn attributed to its persona ([journey
0004](../soul-hq/04-JOURNEY/0004-soulstream-workloads-the-first-agent-runs.md)), a tool answers an agent's
request-reply call ([journey 0005](../soul-hq/04-JOURNEY/0005-soulstream-workloads-a-tool-answers.md)),
and the same declarations run unchanged inside microsandbox microVMs —
backend chosen node-side, constitution III proven ([journey
0007](../soul-hq/04-JOURNEY/0020-soulstream-workloads-a-second-wall.md)). The real-microVM proof is
`make test-msb` (needs `msb` installed); the default gate stays hermetic.
**Phase 2 / M2.1 has landed**: the same declarations run as Kubernetes pods
([journey 0008](../soul-hq/04-JOURNEY/0024-soulstream-workloads-kubernetes-backend.md) research →
[journey 0009](../soul-hq/04-JOURNEY/0028-soulstream-workloads-a-third-wall-lands.md) build; design
[`0002-kubernetes-backend.md`](../soul-hq/02-DESIGN/soulstream-workloads/0002-kubernetes-backend.md)) —
artifact as a per-run OCI image via the operator's registry, credential as
a Secret, runner-supervised pods. The real-cluster proof is `make test-k8s`
(needs `scripts/kind-registry.sh up`); the default gate stays hermetic.
Later horizons (Fleet / sandboxes / tool ecosystem) stay gated (roadmap).

## The rules that bind every change

- **The substrate boundary is non-negotiable** (constitution I): soulstream-workloads is
  never a store of record. Everything worth keeping flows back to soulstream
  topics as ops.
- **Explore → Plan → Code → Commit.** Research goes through `01-RESEARCH/` and
  never through spec-kit; implementation always goes through spec-kit.
- **Quality gate:** `make fmt && make test && make lint` — all green, nothing
  skipped, before any "done" (constitution VI).
- Go module `github.com/impire-io/soulstream-workloads` once implementation starts;
  connect to NATS via `orbit.go/natscontext`, modern `nats.go/jetstream` API;
  never `nats.ws`.
- Sign every commit. Never commit `.claude/settings.local.json`.

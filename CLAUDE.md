# Soulrealm — orientation

**Read [`hq/`](hq/README.md) first.** Everything about how this project is run
lives there: the vision and constitution ([`hq/00-GENESIS/`](hq/00-GENESIS/README.md)),
active research ([`hq/01-RESEARCH/`](hq/01-RESEARCH/README.md)), designs
([`hq/02-DESIGN/`](hq/02-DESIGN/README.md)), the roadmap
([`hq/03-IMPLEMENTATION/`](hq/03-IMPLEMENTATION/README.md)), and the honest log
([`hq/04-JOURNEY/`](hq/04-JOURNEY/README.md)).

## What this is

Soulrealm is the **runtime** of a [soulstream](../soulstream) realm — it
launches, supervises, observes, and retires agents and tools as workloads.
Soulstream is the record; soulrealm is the room.

## Status

**Genesis (2026-07-22).** No code yet, by design. The one open gate is the
substrate research topic:
[`hq/01-RESEARCH/nex-runtime-substrate`](hq/01-RESEARCH/nex-runtime-substrate/README.md).
Nothing is designed or built until it graduates (constitution IV).

## The rules that bind every change

- **The substrate boundary is non-negotiable** (constitution I): soulrealm is
  never a store of record. Everything worth keeping flows back to soulstream
  topics as ops.
- **Explore → Plan → Code → Commit.** Research goes through `01-RESEARCH/` and
  never through spec-kit; implementation always goes through spec-kit.
- **Quality gate:** `make fmt && make test && make lint` — all green, nothing
  skipped, before any "done" (constitution VI).
- Go module `github.com/impire-io/soulrealm` once implementation starts;
  connect to NATS via `orbit.go/natscontext`, modern `nats.go/jetstream` API;
  never `nats.ws`.
- Sign every commit. Never commit `.claude/settings.local.json`.

# Implementation Plan: Launch an agent — the first runtime slice

**Branch**: `001-launch-an-agent` | **Date**: 2026-07-22 | **Spec**: [`spec.md`](spec.md)
**Design**: [`hq/02-DESIGN/0001-soulrealm-runtime.md`](../../hq/02-DESIGN/0001-soulrealm-runtime.md)

## Summary

Soulrealm launches one agent persona onto a single node as a native OS process,
mints it a NATS credential scoped to its persona's realm subjects, and runs it
as the **runner** persona that soulstream's work extension already anticipates
(work.md stage 4 = "stage 2 plus a runner"). The workload's whole visible life
is expressed with the **existing** stage-2 work vocabulary
(`work.open`/`work.claim`/`work.done`/`work.abandon`) on the target topic — so
this slice adds **no new soulstream vocabulary** — and the agent's participation
is an ordinary `turn.post` under its own identity. No second control plane; the
topic op-log carries everything.

## Technical Context

**Language/Version**: Go 1.26
**Primary Dependencies**: `github.com/impire-io/soulstream` (the `topic`,
`record`, `identity`, `realm` packages — soulrealm's *only* domain dependency,
per the soulstream-only scope, episode 0003); `nats-io/nats.go` +
`nats.go/jetstream`; `synadia-io/orbit.go/natscontext`; `nats-io/jwt/v2` and
`nats-io/nkeys` (credential minting).
**Storage**: none of its own (constitution I). Durable state lives in soulstream
topics + object store. Soulrealm keeps only a **local scratch dir** per workload,
reaped on exit.
**Testing**: `go test ./...`. Pure logic (declaration validation, permission-set
construction, credential-claim building, lifecycle→op mapping) unit-tests with
**no server**; integration spins up an **in-process operator-mode NATS** and a
provisioned soulstream realm.
**Target Platform**: macOS + Linux (the native process backend).
**Project Type**: single Go module — library packages + one `cmd`.
**Performance Goals**: N/A for this slice (correctness + observability, not
throughput).
**Constraints**: single node; native backend only; soulstream-only dependencies;
`make fmt && make test && make lint` green with nothing skipped.
**Scale/Scope**: one node, one agent, one topic.

## Constitution Check

*GATE: checked against [`hq/00-GENESIS/constitution.md`](../../hq/00-GENESIS/constitution.md)
(via the `.specify/memory/constitution.md` symlink). Re-check after design.*

- **I. Substrate boundary (NON-NEGOTIABLE)** — PASS. Soulrealm stores nothing
  durable: the execution work item, the agent's turn, and any results are ops on
  the soulstream topic; the only local state is scratch, reaped on exit. No
  package writes a store of record.
- **II. One identity, no privileged tier** — PASS. The agent workload gets a
  freshly minted NATS user scoped to its persona's subjects; soulrealm itself
  acts as an ordinary **runner** persona to publish work ops. Neither is a
  privileged API tier; both are peers on the wire. Behaviour never branches on
  human-vs-machine.
- **III. Contracts orthogonal to backends** — PASS. The `WorkloadDeclaration`
  has no backend field (validation rejects one); the `Backend` interface has a
  single `native` implementation now, Docker later, with the declaration
  unchanged across them.
- **IV. Research gates before build spends** — PASS. The substrate is decided
  (episode 0002); this feature is post-gate and specified from design 0001.
- **V. Execution is observable and attributable** — PASS. Lifecycle is the
  `work.open → work.claim → work.done|abandon` op sequence on the topic,
  attributable to the runner persona, followable and replayable by anyone. No
  private soulrealm control subject exists (SC-002 audits for it).
- **VI. All-green quality gate** — PASS by construction. Pure logic is
  server-free; integration uses an in-process NATS; `make fmt && make test &&
  make lint` is the gate.

No violations → Complexity Tracking is empty.

## Project Structure

### Documentation (this feature)

```text
specs/001-launch-an-agent/
├── spec.md              # the feature spec
├── plan.md              # this file
├── research.md          # the load-bearing technical decisions
├── data-model.md        # entities: declaration, credential, execution work item
├── contracts/
│   ├── interfaces.md     # Minter + Backend Go interfaces; declaration schema
│   └── lifecycle-ops.md  # the lifecycle → soulstream work-op mapping
└── quickstart.md        # run the slice end-to-end (nsc operator + realm)
```

### Source code (repository root — Go module `github.com/impire-io/soulrealm`)

```text
declaration/     # WorkloadDeclaration: parse + validate; reject backend fields. PURE (no NATS).
minter/          # per-persona scoped NATS user minting: permission-set + JWT claim building
                 #   (PURE) + signing with the realm-account key. Reimplements NEX's CredVendor shape.
backend/         # Backend interface (fetch artifact, inject creds env, start, stream lifecycle, stop, reap)
  native/        #   the native OS-process implementation (os/exec supervision)
runner/          # the runner persona: opens/claims the execution work item, launches via Backend,
                 #   emits work.done|abandon on exit — ties minter + backend + the soulstream client.
                 #   The lifecycle→op mapping (PURE) is separated from the NATS I/O.
cmd/soulrealm/   # CLI: `soulrealm workload start <declaration>`
```

**Structure Decision**: Single Go module. The pure surfaces (`declaration`,
`minter` claim-building, `runner` lifecycle→op mapping) hold no NATS import and
unit-test without a server — mirroring soulstream's own record/identity split.
The NATS-touching code (`minter` signing/publish, `backend/native`, `runner`
I/O, `cmd`) is validated against an in-process operator-mode server.

## Complexity Tracking

No constitution violations; no entries.

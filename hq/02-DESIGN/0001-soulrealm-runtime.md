# 0001 — The soulrealm runtime

**Status of this document:** first architecture doc, graduated from the
`nex-runtime-substrate` research topic (episode 0002). It fixes the *shape* of
the runtime and the decided principles; it marks with `[O]` the sub-questions
that still need their own design or research pass. Written functional-level so
a later `/speckit-specify` can turn a slice of it into a feature spec.

Maturity tags (per [`README.md`](README.md)): **[D]** designed, not yet built;
**[O]** interface + default named, best internal still open.

---

## 1. What the runtime is

Soulrealm launches, supervises, observes, and retires a realm's **workloads** —
the agents and tools of a [soulstream](../../../soulstream) realm — and does so
with the soulstream topic op-log as its **single control plane**. There is no
second coordination system: a workload's whole visible life (requested,
placed, started, progressing, produced, exited) is *operations on topics*, the
same records humans and agents already read. This is the decision that chose a
from-scratch runtime over embedding NEX (episode 0002): eliminating NEX's
separate control plane (`$NEX.control.*`, auctions, `$NEX.agent.*`) in favour of
one plane the realm already trusts.

NEX is **design influence, not a dependency.** Where NEX solved a problem well,
soulrealm borrows the *shape* (named in §7) and reimplements it against the
op-log, rather than importing its machinery and bridging two planes.

**Dependency scope (decided 2026-07-22, episode 0003):** soulrealm depends on
**soulstream only**. It provisions nothing of the wider Impire platform and
takes no dependency on its services (identity, tenancy, vault) for now.
Everything the runtime needs — realm, topics, personas, object store, and the
account it mints credentials under — is soulstream's surface plus a
soulrealm-held signing key. A future hand-off of signing authority to an
external platform service stays *possible* through the minter seam (§4), but is
explicitly not designed in now.

## 2. Principles this runtime is held to

Straight from the constitution; every later section serves these.

- **I — Substrate boundary.** The runtime is never a store of record. A
  workload's authoritative artefacts, history, and state live in topics; the
  runtime holds only scratch. A dead workload loses scratch, never history.
- **II — One identity, no privileged tier.** Every workload runs as a persona
  with scoped NATS credentials, minted per workload, scoped to that persona's
  realm subjects — never a shared or elevated identity.
- **III — Contracts orthogonal to backends.** A workload's declaration says
  nothing about the isolation backend that runs it; the backend is a node-side
  choice, swappable without editing the declaration.
- **V — Execution is observable and attributable.** Lifecycle is ops on the
  topic, or the work is not done. No black-box execution.

## 3. The workload model: two orthogonal axes

The `nex-runtime-substrate` research established (measured) that *role* and
*lifecycle* are independent. Soulrealm keeps both, explicitly.

**Role — what the workload is to the realm** (soulrealm's own axis):

- **`agent`** `[D]` — a long-lived persona that *participates*: it holds a
  persona identity, follows and posts to topics, claims and completes work
  items. An agent is a first-class member of the realm, not a service behind an
  API (constitution II).
- **`tool`** `[D]` — a *capability other workloads call*: an MCP server, a
  code/exec sandbox, a data connector. A tool is addressed and invoked over the
  realm transport under the same identity model; it is discoverable, not
  privileged.

**Lifecycle — how the runtime schedules it** (borrowed from NEX's axis):

- **`service`** `[D]` — long-lived; runs until stopped.
- **`function`** `[D]` — short-lived; triggered on demand, exits when done.
- **`job`** `[D]` — runs to completion once (batch/scheduled).

The axes are independent: an `agent` is nearly always a `service`; a `tool` may
be a persistent `service` (an always-on MCP server) *or* a `function` (spun up
per call). The declaration carries both; nothing collapses one into the other.

## 4. Identity and authorization

Each workload gets a **freshly minted, per-workload NATS user**, scoped to the
subjects its persona is allowed to touch — never a shared credential
(constitution II). The design is influenced directly by NEX's minter
(`models.CredVendor` + xkey-encrypted env delivery), reimplemented as
soulrealm's own:

- **Minting** `[D]` — a soulrealm minter issues a user keypair + JWT per
  workload, signed under the realm's account, with `Permissions` scoped to that
  persona's soulstream subjects (e.g. publish `SOULSTREAM.TOPICS.OPS.<topics
  the persona works>`, its own inbox, the object store it may read/write).
  Unlike stock NEX, the scope is *realm-semantic* from the start.
- **Delivery** `[D]` — credentials reach the workload through its environment.
  For a **local** process soulrealm forks itself (single node) there is no
  untrusted intermediary, so the env is injected directly (refined by spec 001
  research D4). The **xkey-encrypted environment** (NEX's mechanism, kept) is
  the delivery for when a start request travels over NATS to a node soulrealm
  does not control — a multi-node concern, added with multi-node.
- **Trust** `[O]` — the realm's NATS account must trust the soulrealm signing
  key. Operator-mode provisioning (which account signs, how the key is held per
  node vs central) is an open sub-question, scoped to soulstream + soulrealm:
  soulrealm holds a realm-account signing key (dev: provisioned with `nsc`).
  The minter is a seam, so an external signing authority could take over later
  without changing the workload contract — but no such external dependency is
  designed in now (see §1 dependency scope).

## 5. Lifecycle as ops (the single control plane)

Every stage of a workload's life is an operation on a topic, following the
soulstream work-extension vocabulary
([work.md](../../../soulstream/hq/02-DESIGN/extensions/work.md), stage 4):

- A workload is **requested** as a work item (`work.open` flavoured for
  execution) on a topic. `[D]`
- The runtime **claims** it (stream-order-wins, the house rule — no lock
  service), then emits **placed / started / progress / result / exited** as ops
  on that topic. `[D]`
- Results (artefacts, logs pointers) flow back as `attachment.add` / baseline
  ops; the object store holds bytes, the op-log holds history. `[D]`
- **Placement / scheduling** across more than one node `[O]` — NEX uses an
  auction. Soulrealm's placement must be expressed as ops on the plane too
  (a claim race, or a designed auction-in-ops); the mechanism is open and
  deferred until more than one node is real.

There is no `$NEX.control.*` analogue. If a piece of coordination cannot be
expressed as ops a persona can read, that is a design smell to resolve, not a
reason to add a second plane.

## 6. Isolation backends (orthogonal, pluggable)

The contract in §3 says nothing about *how* a workload is isolated. A node
selects a backend; the declaration is unchanged across them (constitution III).

- **native process** `[D]` — first backend; the reference the others are
  validated against.
- **Docker/OCI** `[D]`, **Firecracker microVM** `[O]`, **Kubernetes pod** `[O]`
  — each a backend plugin behind one interface: fetch the artefact, inject the
  xkey-encrypted creds/env, start, stream lifecycle as ops, stop, reap.
- The backend interface is soulrealm's, shaped by NEX's nexlet/agent SDK
  (artifact fetch via object store `nats://`, env-decrypt on the workload side)
  but owned here so it emits ops rather than NEX control messages.

## 7. What is borrowed from NEX (influence ledger)

Recorded so the influence is honest and the reversal condition (episode 0002)
is checkable — if we end up reimplementing all of this at NEX's scope, the
rebuild bet was wrong.

| NEX idea | How soulrealm uses it |
|---|---|
| Per-workload minted scoped NATS user (`CredVendor`) | Reimplemented as a realm-semantic minter (§4) |
| xkey-encrypted env delivery | Kept as-is in shape (§4) |
| Runtime/nexlet SDK (artifact fetch, env decrypt, run) | Shapes the backend interface (§6) |
| Role-free `service`/`function`/`job` lifecycle | Adopted as the lifecycle axis (§3) |
| Auction placement | Reference for the `[O]` op-plane placement (§5) |

What is **not** borrowed: NEX's control plane, its node/agent registration
protocol, its `$NEX.*` subject space. Those are exactly what the second-plane
decision rejects.

## 8. Out of scope for the first build

Named so their absence is deliberate; each has a seam.

- **Multi-node placement / scheduling** `[O]` — first build is single-node; the
  claim-in-ops seam (§5) is where it grows.
- **Firecracker / Kubernetes backends** `[O]` — native + Docker first; §6 is the
  seam.
- **Sandboxes** (soulstream work stage 5) — gated on stage-4 execution being
  real first; a later design doc.
- **Tool discovery/marketplace** — the `tool` role runs; rich discovery is
  later vocabulary.

## 9. Acceptance criteria for the first slice

The first spec-kit feature out of this doc should demonstrate, on one node,
against an operator-mode NATS realm:

1. An `agent` (role) / `service` (lifecycle) workload is declared, minted a
   persona-scoped credential, launched under the native backend, and **posts a
   turn to a topic as its persona** — attribution verifiable on the op-log.
2. Its lifecycle (started / progress / exited) appears as **ops on the topic**,
   readable by any persona (constitution V), with no second control plane.
3. The workload's declaration contains **no backend-specific field**
   (constitution III), proven by the Docker backend running the same
   declaration unchanged in a follow-on slice.

These map to roadmap Phase 1 (M1.1–M1.3); the exact op subjects, the
declaration schema, and the minter's signing story (§4 `[O]`) are the first
questions `/speckit-specify` + `/speckit-plan` must pin down.

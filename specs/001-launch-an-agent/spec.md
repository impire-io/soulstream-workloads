# Feature Specification: Launch an agent — the first runtime slice

**Feature Branch**: `001-launch-an-agent`
**Created**: 2026-07-22
**Status**: Draft
**Input**: Roadmap Phase 1 M1.1, from design [`hq/02-DESIGN/0001-soulrealm-runtime.md`](../../hq/02-DESIGN/0001-soulrealm-runtime.md) §9.

Soulrealm launches a single **agent** persona onto one node, minted a
persona-scoped NATS credential, running under the **native** backend,
participating in a soulstream topic under its own identity — with its whole
visible life expressed as operations on a topic and no second control plane.
This is the vertical slice that proves the four load-bearing constitution
articles (I substrate boundary, II one identity, III contracts orthogonal to
backends, V observable execution) end to end on the smallest real workload.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Launch an agent that participates as its persona (Priority: P1)

An operator has a provisioned soulstream realm and wants an agent to take part
in it. They write a workload declaration — role `agent`, lifecycle `service`, a
persona name, a target topic, and the artifact to run — and hand it to
soulrealm. Soulrealm mints a NATS credential scoped to that persona, launches
the artifact as a native process on the node, and the running agent posts a
turn to the topic. The turn appears on the topic's op-log **attributed to that
persona**, indistinguishable in kind from a human persona's turn.

**Why this priority**: This is the MVP. On its own it demonstrates the whole
thesis — identity issuance, native launch, and first-class participation — in
one runnable slice. Nothing smaller proves the runtime exists.

**Independent Test**: Provision a dev realm (operator-mode NATS + SOULSTREAM
stream), declare one agent bound to a topic, run the launch command, then follow
the topic and confirm a turn attributed to the declared persona appears — with
no manual credential handling by the operator.

**Acceptance Scenarios**:

1. **Given** a provisioned realm and a valid agent declaration, **When** the
   operator launches it, **Then** soulrealm mints a persona-scoped credential,
   starts the native process, and a turn posted by the agent appears on the
   topic attributed to the declared persona.
2. **Given** a running agent, **When** a follower materialises the topic,
   **Then** the agent's turn is present and verifies as authored by the persona
   (attribution intact on the op-log).
3. **Given** a declaration naming a persona the realm account cannot sign for,
   **When** the operator launches it, **Then** soulrealm refuses to start and
   surfaces the failure — no process, no partial state.

---

### User Story 2 - See a workload's life as ops (Priority: P2)

Any persona in the realm can watch a workload live without special tooling: its
lifecycle transitions are ordinary operations on a topic. As the agent is
requested, starts, runs, and exits, those transitions show up on the topic's
op-log, readable and replayable like any other operation — and there is no
private soulrealm control subject carrying this out of band.

**Why this priority**: Constitution V (observable execution) and the whole
single-control-plane bet. Without it the runtime is a black box; with it,
execution is as legible as conversation.

**Independent Test**: Subscribe to the topic and to a wildcard of everything
*outside* the realm's op subjects; launch and then kill the agent; confirm the
lifecycle transitions appear as ops on the topic and nothing soulrealm-private
appears on any other subject.

**Acceptance Scenarios**:

1. **Given** an agent being launched, **When** it transitions
   (requested → started → exited), **Then** each transition is published as an
   operation on the topic, in order, attributable to the runtime persona.
2. **Given** a running agent, **When** its process is killed, **Then** an
   `exited` operation carrying the outcome appears on the topic and its scratch
   and credential are reaped.
3. **Given** the agent's full lifecycle, **When** an observer audits all
   subjects, **Then** no soulrealm coordination traffic exists on any subject
   outside the realm's documented op/inbox/object-store space.

---

### User Story 3 - A declaration that doesn't know its backend (Priority: P3)

The operator's declaration says *what* to run and *as whom*, never *how* it is
isolated. The native backend is chosen on the node, not in the declaration, so
the same declaration will later run under Docker unchanged.

**Why this priority**: Constitution III. Proving it fully needs the second
backend (M1.3); this slice guarantees the *declaration* is already
backend-agnostic so that proof is later possible without a schema change.

**Independent Test**: Validate that the declaration schema has no
backend-specific field and that a declaration carrying one is rejected; record
the same declaration for re-use unchanged by the M1.3 Docker backend.

**Acceptance Scenarios**:

1. **Given** a declaration with a backend-specific field, **When** it is
   validated, **Then** it is rejected with a clear error.
2. **Given** a valid declaration, **When** the node runs it, **Then** the node
   selects the native backend without any backend hint in the declaration.

### Edge Cases

- **Unknown / unsignable persona** — mint fails; launch is refused with an
  observable error; no process starts (US1 scenario 3).
- **Artifact fetch fails** — no process; a failure op on the topic; no
  half-registered credential left behind.
- **Workload exits non-zero or crashes** — an `exited` op records the outcome;
  service restart policy for this slice is *record and stop* (bounded
  auto-restart is a named later feature, not built here).
- **Realm NATS unreachable at launch** — launch fails cleanly; no partial
  state; error surfaced to the operator.
- **Credential outlives the workload** — minted credentials are bounded
  (expiry) and reaped on exit; a dead workload cannot keep publishing.
- **Two nodes race for the same declaration** — out of scope (single node in
  this slice); the claim-in-ops seam handles it when multi-node lands.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: soulrealm MUST accept a workload declaration specifying role
  `agent`, lifecycle `service`, a persona reference, a target topic, and the
  artifact to run, and MUST reject any backend-specific field (constitution
  III).
- **FR-002**: soulrealm MUST mint a per-workload NATS user scoped to the
  declared persona's realm subjects — publish the topic's op subject and the
  persona's inbox, read/write the realm object store for attachments — signed
  under the realm account; never a shared or elevated credential (constitution
  II).
- **FR-003**: soulrealm MUST deliver the minted credential to the workload
  without exposing it in plaintext to intermediaries (xkey-encrypted
  environment, per the NEX-influenced delivery in design 0001 §4).
- **FR-004**: soulrealm MUST launch the workload as a native OS process on the
  node (the native backend).
- **FR-005**: The launched agent MUST be able to post a turn to the declared
  topic such that the operation is attributable to the declared persona on the
  op-log, verifiable by any follower.
- **FR-006**: soulrealm MUST publish the workload's lifecycle transitions (at
  minimum: requested, started, exited) as operations on a topic, following the
  soulstream work-extension execution vocabulary — with no separate private
  control subject (constitution V).
- **FR-007**: soulrealm MUST NOT be the store of record for any workload
  output; artefacts and history flow back to the topic as ops and object-store
  attachments (constitution I). A dead workload loses only scratch.
- **FR-008**: When a credential cannot be minted (unknown persona, account
  cannot sign), soulrealm MUST refuse to launch and surface an observable
  error — no silent partial start.
- **FR-009**: soulrealm MUST obtain its signing authority (the realm-account
  signing key + root account public key) through a single named seam. The seam
  resolves to a **soulrealm-held key**; it MUST be shaped so an external signing
  authority could take over later **without changing the workload contract**,
  but no such external dependency is designed in now (soulstream-only scope).
- **FR-010**: Minted credentials MUST be bounded (expiry) and reaped when the
  workload exits; lifecycle ops MUST be emitted for both normal and abnormal
  exit.

### Key Entities *(include if feature involves data)*

- **Workload declaration** — role, lifecycle, persona ref, topic ref, artifact
  URI. Carries no backend field. The operator-facing contract.
- **Minter** — mints per-persona scoped NATS users signed under the realm
  account; fronts the signing-authority seam (soulrealm-held; a future external
  authority stays possible but is not built). Realm infrastructure, not a
  persona.
- **Persona-scoped credential** — a user JWT + seed whose permissions are
  limited to the persona's realm subjects; delivered xkey-encrypted; bounded
  lifetime.
- **Native backend** — fetches the artifact, injects the encrypted credential
  env, runs the process, streams lifecycle as ops, reaps on exit. One
  implementation of the backend seam (constitution III).
- **Execution lifecycle ops** — the topic-anchored record of the workload's
  life (requested/started/exited …), expressed in the work-extension
  vocabulary. The single control plane.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator launches an agent with a single command and sees its
  posted turn attributed to the declared persona on the topic, having handled no
  credentials by hand.
- **SC-002**: Following the topic shows the workload's lifecycle
  (started → … → exited) as ops, and an audit of all other subjects shows zero
  soulrealm-private control traffic.
- **SC-003**: The minted credential cannot publish outside the persona's scoped
  subjects — an attempt to publish an unrelated subject is denied by the server.
- **SC-004**: Killing the workload process yields an `exited` op on the topic
  and leaves no orphaned scratch or live credential.
- **SC-005**: The declaration contains no backend-specific field — a schema
  check passes and a declaration carrying one is rejected.

## Assumptions

- A soulstream realm is already provisioned and reachable (SOULSTREAM stream,
  object store, persona directory). Soulrealm runs *on* a realm; it does not
  provision one.
- The realm's NATS runs in **operator mode** with a resolver that trusts the
  realm account's signing key, set up with `nsc` (the shape of nex's
  `_examples/operator_mode`).
- soulrealm **holds the realm-account signing key directly.** Scope for this
  and every near-term slice is **soulstream + soulrealm only** — no dependency
  on the wider Impire platform (identity, tenancy, vault). A future hand-off of
  signing authority to an external service stays possible through the minter
  seam (FR-009) but is not designed in now.
- The exact op names for execution lifecycle (requested/started/exited …) are
  pinned down in `/speckit-plan` against soulstream's work-extension vocabulary;
  they extend, not fork, that vocabulary.
- The agent artifact is a program that uses the soulstream client to post a
  turn; building rich agent runtimes is later work.
- Single node. Multi-node placement/scheduling, the `tool` role (M1.2), and a
  second isolation backend (M1.3) are out of scope for this feature.

# Feature Specification: Agent declaration — wake, instructions, capabilities

**Feature Branch**: `009-agent-declaration`
**Created**: 2026-08-25
**Status**: Draft
**Input**: soul-hq design [`0005-agent-declaration.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0005-agent-declaration.md) (graduated research, episode 0126), with the wake-budget admission of design [`0006-loop-safety.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0006-loop-safety.md) (built here as specs/008) mandatory on every wake kind. The declaration grows wake entries (mention, topic, schedule, subject), a record-form artifact, record-borne instructions, and declared capability names; the wrap engine generalizes its measured mention protocol to all four kinds. Growth is schema and provisioning, never machinery: the SOULSTREAM_SYSTEM stream (core f0a09f2) already carries schedules; no new op types, no core wire change.

## Fixed decisions (made before this spec; recorded, not relitigated)

### The enforcement-read gap — RESOLVED: runtime-side reads (design 0005 §5 [O])

The wake engine performs **every** record-position read — inbox catch-up, topic
materialisation, outcome-existence checks, budget computation, tick
consumption, instructions materialisation — with the **engine's own
connection and credential**. In wrap that is the agent's paste-block
credential exactly as today; in a future fleet dispatcher it is the runtime
persona's connection. The workloads minter's agent scope
(`minter/scope.go` — `$JS.API.INFO` only) is **not** widened.

Rationale, recorded: JetStream read-API subjects (`$JS.API.STREAM.*`,
`$JS.API.CONSUMER.*`) are stream-wide tails — widening the agent template
with them breaches own-prefix confinement (an agent could read every
persona's inbox and every topic through the JS API even where subject
subscription is denied). And scoped templates are written into the account
at founding: widening the template is a per-deployment control-plane
migration (the byon rc.10 lesson). Runtime-side reads keep the agent scope
frozen and put the read authority where the admission decision already
lives.

### Capabilities are schema-only this slice (design 0005 §5, fixed decision)

`capabilities {role, tools[]}` parses and validates strictly and is
documented as **names, not grants**: `role` names a vault entry (a scoped
signing key, identity design D28's selector), `tools` ride as
`tool:<name>` mint tags for the account's scoped template to resolve at
the transport. The identity-backed Minter that resolves these names via
D28 `mint.ephemeral` is the **named follow-on feature
`capability-minting`** — this repo deliberately carries no
soulstream-identity dependency since the central-daemon cut (design 0004
§9), and this slice does not reintroduce one. See Out of Scope.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The record declares what wakes the agent (Priority: P1)

An operator writes one declaration for an agent persona: it answers
mentions, watches a topic, runs on a schedule, and listens on a subject.
The wake engine drives all four from that one document — each wake ends in
exactly one outcome under the deterministic wake op-id, and the record
itself is the position: a restart mid-backlog answers nothing twice for
any stream-backed kind. The subject kind is honestly at-most-once: a wake
arriving while the engine is down is lost, and declaring the kind is
declaring that.

**Why this priority**: This is the feature — the roadmap's named hole (the
declaration trigger vocabulary) and design 0005's acceptance bar 1.

**Independent Test**: One declaration with all four kinds against an
embedded realm; fire each kind; restart the engine; assert stream count
and attempt count both 1 per stream-backed trigger, and the down-time
subject publish produced nothing.

**Acceptance Scenarios**:

1. **Given** a declaration with mention, topic, schedule, and subject wake
   entries, **When** each source fires once, **Then** exactly one outcome
   per trigger lands under `WakeOpID(trigger identity, persona)` —
   mention/topic outcomes in the triggering topic, schedule/subject
   outcomes on the declared home topic.
2. **Given** answered wakes and a backlog, **When** the engine restarts,
   **Then** no trigger is answered twice (outcome count per trigger stays
   1) and no harness is re-invoked for an answered trigger (attempt count
   stays 1).
3. **Given** a declared subject wake and a stopped engine, **When** a
   message is published on the subject and the engine then starts,
   **Then** no wake fires for it — the loss is the documented delivery
   class, not a bug.

---

### User Story 2 - Instructions live in the record (Priority: P1)

An agent's instructions are a stage-1 artefact lineage on a topic. The
engine materialises the lineage **tip** at every wake, digest-checked,
holds no durable copy, and delivers the text to the harness through the
prompt fill. Revising the artefact through ordinary ops reprograms the
agent's next wake with no redeploy.

**Why this priority**: The declare flow's whole point — the agent is run
from the record, not from host files (design 0005 §2 [V]).

**Independent Test**: Declare instructions; wake; revise the artefact via
`topic.Revise`; wake again on the same running engine; the second prompt
carries the revision.

**Acceptance Scenarios**:

1. **Given** a declaration with `instructions {topic, artefact}` and
   revision v1 attached, **When** a wake runs, **Then** the harness prompt
   contains v1's text.
2. **Given** the same running engine and a revision v2 posted through
   ordinary ops, **When** the next wake runs, **Then** the prompt contains
   v2 — no restart, no redeploy, and nothing but the run's scratch dir was
   written.

---

### User Story 3 - Colonies stay safe: self-exclusion and the budget on every kind (Priority: P1)

A topic wake never fires on ops authored by the declared persona itself
(normative — the loop appears on day one without it), and **every** wake
kind passes the 0006 wake budget at the same admission seam: after the
self-skip and the outcome-existence pre-check, before the harness.
Refusals stay op-less and loud.

**Why this priority**: Design 0005 §7's sequencing rule — topic-wake
colonies unblock only with the budget in the admission path of whatever
dispatches them (0006 §6).

**Independent Test**: Two declared agents topic-waking each other's turns
halt at the budget with `wake_refused` log lines and zero refusal ops; an
op authored by the declared persona produces no wake at all.

**Acceptance Scenarios**:

1. **Given** an agent with a topic wake on topic T, **When** the agent
   itself posts a turn in T (through its own client, any op id), **Then**
   no wake fires for that op.
2. **Given** two declared agents whose topic wakes cover each other's
   outcomes, **When** a person posts one turn, **Then** the cascade halts
   at the budget bound with at least one loud `wake_refused` line and no
   refusal op of any kind in the topic.

---

### User Story 4 - The artifact can live in the record (Priority: P2)

A declaration's `artifact` may name a record artefact —
`soulstream://<topic-path>/<artefact-name>` — beside the existing
`file://` form. The runner resolves the record form by materialising the
lineage tip into the run's scratch directory, digest-checked, never a
durable copy; `file://` behaves exactly as today.

**Why this priority**: The registration-points-at-an-artifact trigger
named by design 0004 §10 (harness-as-workload); required for the declare
flow's "boot from the registration read out of the record".

**Independent Test**: Launch a declaration with a `soulstream://` artifact
against an embedded realm; the workload runs the attached executable from
scratch; a digest mismatch refuses the launch.

**Acceptance Scenarios**:

1. **Given** an artefact lineage whose tip is an executable, **When** a
   declaration pointing at it launches, **Then** the workload runs from a
   scratch-dir copy and the scratch dir is reaped on exit.
2. **Given** a `file://` declaration, **When** it launches, **Then**
   behavior is byte-for-byte today's.

---

### User Story 5 - Capabilities are declared names (Priority: P3)

`capabilities {role, tools[]}` parses and validates: `role` a valid
soulstream name (the vault entry selector), each tool a valid name. The
declaration cannot widen anything — the names resolve elsewhere (the
`capability-minting` follow-on).

**Independent Test**: Table-driven parse/validate cases; a declaration
with capabilities on a non-agent role refuses.

**Acceptance Scenarios**:

1. **Given** `capabilities {role: "agent-default", tools: ["web-fetch"]}`
   on an agent declaration, **When** it parses, **Then** validation
   passes and the fields are available to the runtime.
2. **Given** an invalid role or tool name, or capabilities on a tool
   declaration, **When** validated, **Then** the declaration refuses with
   a teaching error.

---

### User Story 6 - Nothing existing moves (Priority: P1)

A declaration without the new fields, and wrap's existing mention-only
configuration path, behave exactly as today — the 0006 precedent (budget
0/0 reproduces prior behavior) extended to the whole growth.

**Independent Test**: The existing declaration, wrap, and integration
suites pass unmodified (except tests that enumerate declaration fields).

**Acceptance Scenarios**:

1. **Given** a pre-009 declaration document, **When** parsed and
   validated, **Then** the result is identical to today's.
2. **Given** a wrap config with no wake set and no declaration, **When**
   the wrapper runs, **Then** it is mention-only, byte-for-byte today's
   engine.

---

### Edge Cases

- One op that both mentions the persona and matches a topic wake derives
  the **same** wake op-id from both paths (the trigger identity is the
  same op id): one outcome slot, answered once — the identity collapse is
  correct behavior, not a race.
- A schedule tick that expires (per-message TTL) before the engine reads
  it never wakes: the TTL-bounded backlog is the declared delivery class.
- Re-publishing a schedule registration replaces the schedule (no double
  ticks); purging the registration subject deregisters it.
- A topic wake's catch-up is the whole ops history of the declared path:
  outcome-existence is the position, so an old topic's unanswered
  matching ops are a real backlog — the window budget is the throttle.
- Schedule and subject wakes have no author: the self-skip cannot match,
  the depth walk reads the trigger as a chain root, and the failure
  self-report taps nobody (there is no asker) — it posts on the home
  topic without mentions.
- A parked wake (realm unreachable mid-wake) posts nothing; mention and
  topic wakes are re-found by catch-up/reconnect, schedule wakes by the
  next engine start's backlog replay, and a parked subject wake is lost —
  its delivery class.
- An instructions lineage that cannot be materialised (missing artefact,
  digest mismatch) fails that wake loudly without posting; the trigger
  stays answerable.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The declaration MUST grow optional fields `instructions
  {topic, artefact}`, `capabilities {role, tools[]}`, `wake [{kind, …}]`,
  and `budget {max_hops, window {max, per}}`, all strict-decoded (unknown
  fields refuse the document, nested included) and valid only for
  `role: agent`.
- **FR-002**: `artifact` MUST accept the record form
  `soulstream://<topic-path>/<artefact-name>` beside `file://`; the topic
  path validates by the existing path grammar and the artefact name MUST
  be non-empty and slash-free. `file://` validation and `ArtifactPath()`
  are unchanged.
- **FR-003**: Wake entries MUST validate per kind — `mention` (no
  fields), `topic` (`path` required, `types[]` defaulting to
  `[turn.post]`), `schedule` (`name` a valid soulstream name, `pattern`
  matching `@every <Go duration>` / `@at <RFC3339 UTC>` / 6-field cron,
  `ttl` an optional positive duration), `subject` (`subject` a valid
  plain NATS subject) — refusing fields foreign to the kind and duplicate
  entries (one mention entry, unique topic paths, unique schedule names,
  unique subjects).
- **FR-004**: Each wake kind MUST carry its delivery class as a normative
  fact readers and shells surface: mention = replay-exact (notify stream,
  inbox-window bounded); topic = replay-exact (ops stream); schedule =
  replay-exact, TTL-bounded backlog; subject = at-most-once.
- **FR-005**: Every wake outcome MUST publish under
  `WakeOpID(trigger identity, persona)` on the frozen UUIDv5 namespace,
  with trigger identities exactly: mention = the notify op id (unchanged);
  topic = the triggering op's id; schedule = the tick's SOULSTREAM_SYSTEM
  stream sequence as a decimal string; subject = the lowercase-hex SHA-256
  digest of the message payload.
- **FR-006**: Record wakes (mention, topic) MUST answer in the topic
  where they were triggered; non-record wakes (schedule, subject) MUST
  land their outcomes on the declared home topic (the declaration's
  `topic` field).
- **FR-007**: The failure self-report MUST tap only the asker for mention
  wakes (unchanged) and the triggering op's author for topic wakes; for
  schedule and subject wakes it MUST post on the home topic with no
  mentions.
- **FR-008**: A topic wake MUST exclude ops authored by the declared
  persona (normative self-exclusion), and MUST fire only on ops whose
  type is in the entry's declared types.
- **FR-009**: Every wake kind MUST pass the 0006 budget admission at the
  same seam order handleWake uses today — self-skip → outcome-existence
  pre-check → budget → invoke → discharge — with refusals op-less and
  loud (`wake_refused` with the numbers).
- **FR-010**: All record-position reads MUST be performed with the
  engine's own connection (the runtime-side-reads decision above); no
  agent-scope widening, no durable consumers — outcome existence against
  the topic is the only position for stream-backed kinds.
- **FR-011**: Reconciling a declaration's schedule entries MUST be
  publishing one headered registration message per entry to
  `SOULSTREAM.SYSTEM.SCHEDULES.<persona>.<name>` (`Nats-Schedule` =
  pattern, `Nats-Schedule-Target` = the TICKS subject,
  `Nats-Schedule-TTL` = the entry's ttl when set); re-publish replaces,
  purge deregisters. Tick consumption MUST be engine-side and
  non-durable.
- **FR-012**: Declared instructions MUST be materialised at every wake —
  lineage tip, digest-checked, in memory only (no durable copy) — and
  delivered via the prompt fill (`{{INSTRUCTIONS}}`); a revision through
  ordinary ops MUST reach the next wake with no redeploy.
- **FR-013**: A `soulstream://` artifact MUST be resolved by the runner
  through an artifact-source seam: lineage tip fetched from the record,
  digest-verified, written into the run's scratch dir (reaped with it);
  `file://` resolution is unchanged and a record-form artifact without a
  configured source refuses before any op publishes.
- **FR-014**: The declaration's `budget` block MUST map onto wrap's
  existing Budget knobs (defaults MaxHops 4, WindowMax 8, WindowPer 10m;
  zero parts disabled; `Unbudgeted` the explicit opt-out).
- **FR-015**: `soulstream-wrap` MUST accept a declaration file whose
  agent wake entries drive the engine (persona must match the resolved
  credential's persona); without one, mention-only operation is
  byte-for-byte today's. The engine stays host-agnostic — a future fleet
  dispatcher consumes the same package and inherits the admission seam.

### Key Entities

- **Wake entry**: one declared wake source — kind, per-kind fields, its
  delivery class, and (through the engine) its trigger-identity rule.
- **Trigger identity**: the string hashed with the persona into the wake
  op-id; per kind as FR-005. The record's one outcome slot per wake.
- **Home topic**: the declaration's `topic` — the outcome home for
  non-record wakes and the lifecycle topic as today.
- **Instructions reference**: `{topic, artefact}` naming a stage-1
  artefact lineage; the tip is the agent's current instructions.
- **Record-form artifact**: `soulstream://<topic-path>/<artefact-name>`
  — the executable's lineage in the record; resolved per run into
  scratch.
- **Capability names**: `{role, tools[]}` — selectors for the identity
  plane, granting nothing by themselves.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Four wake kinds fire from one declaration; every
  stream-backed wake answers exactly once across an engine restart —
  outcome count per trigger 1 in the stream, harness attempt count per
  trigger 1 on disk.
- **SC-002**: A subject publish while the engine is down produces no wake
  after restart (asserted, documented at-most-once).
- **SC-003**: An instructions revision through ordinary ops appears in
  the very next wake's prompt on a running engine; no file outside the
  run scratch is written.
- **SC-004**: An op authored by the declared persona never wakes its own
  topic wake (zero invocations).
- **SC-005**: An uncooperative two-agent topic-wake cycle halts at the
  budget bound with ≥1 loud op-less refusal and zero refusal ops.
- **SC-006**: A schedule registration produces ticks that wake the agent
  with outcomes on the home topic; a re-published registration does not
  double the tick rate; ticks older than the declared ttl are expired
  from the stream (the TTL-bounded backlog, measured).
- **SC-007**: The pre-009 test suites pass unmodified except where they
  enumerate declaration fields.

## Out of Scope (named follow-ons)

- **`capability-minting`**: the identity-backed Minter resolving declared
  capability names via identity D28 `mint.ephemeral` tag templates. This
  slice ships schema + validation only; no soulstream-identity dependency
  is introduced (deliberate since the daemon cut).
- The fleet dispatcher itself (design 0003/0005 §9): the engine is built
  host-agnostic for it, but no dispatcher lands here.
- The declare-flow shell verb and attestation steps (design 0005 §8) —
  product composition.
- Soul-topic posting-rights scoping and pinned instruction digests
  (design 0005 §6) — the guarded-surface policy lives with the identity
  plane; a registration MAY later pin a digest.
- Runtime join/leave without restart (watched reversal, unfired).

## Assumptions

- SOULSTREAM_SYSTEM exists wherever the engine runs — core f0a09f2
  provisions it; rigs call `Provision` (create-or-report).
- nats-server 2.14.3 message-schedule semantics as measured by core's
  gate: a headered registration appends ticks (~1.3s cadence for
  `@every 1s`), the tick carries the registration's payload, no consumer
  needed; `Nats-Schedule-TTL` stamps the scheduler's ticks with a
  per-message TTL (verified live by this feature's tests — a divergence
  stops the schedule slice and is recorded, not papered over).
- Ticks and subject messages are non-record plumbing: unsigned,
  server-generated or caller-published, never authoritative — the
  outcome op in the record is the only truth a wake leaves.
- A topic wake's paths are within the engine credential's read scope
  (the runtime-side-reads decision); a path outside it fails loudly at
  subscribe/materialise time.
- The window budget is the throttle for deep topic backlogs; operators
  declaring a topic wake on a long topic accept replay-exact semantics.

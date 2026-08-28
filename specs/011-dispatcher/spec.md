# Feature Specification: The standing dispatcher — agents as infrastructure

**Feature Branch**: `011-dispatcher`
**Created**: 2026-08-28
**Status**: Draft
**Input**: soul-hq design [`0007-agents-as-infrastructure.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0007-agents-as-infrastructure.md) (graduated research, episode 0141), composing design [`0003-fleet.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0003-fleet.md)'s placement plane (built, `fleet`), design [`0004-wrap.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0004-wrap.md) §9's promised serve arm, and design [`0006-loop-safety.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0006-loop-safety.md)'s admission budget (built, specs/008, already ridden by the wake engine of specs/009). A standing process per fleet node watches the placement topic, races open placements through the ordinary claim path, and serves each agent placement it wins by running the shipped wake engine. Nothing new is invented: no coordinator, no consumer state beside the log, no new realm vocabulary.

## Fixed decisions (made before this spec; recorded, not relitigated)

### The serve seam — RESOLVED: the dispatcher owns its claim path (design 0007 §2 [O], option **(b)**)

`fleet.Node.TryPlace` hardwires `Runner.Launch` — correct for backend
workloads, wrong for engine-served agents. Of the two candidates the
design named, this feature takes **(b)**: the dispatcher publishes its
own `ClaimWork` → re-materialise → serve-on-read-back, and composes
`fleet.Node`'s probe/sweep halves beside it. **`fleet` gains no launch
hook and no serve closure.**

Rationale, recorded:

- The probe and sweep halves of `fleet.Node` already work standalone —
  `Sweep` needs `ID`, `Conn`, and the two bounds, never a `Runner`
  (measured in the graduation spike). Option (a) would have made every
  fleet node carry a launch function it does not use.
- The claim path is eleven lines of ordinary ops. Duplicating it costs
  less than widening `fleet.Node`'s contract for one caller, and it
  keeps the reclaim discipline (§6 of 0003) literally untouched: the
  dispatcher's claims and abandons are the same ops a `TryPlace` node
  publishes, so a dispatcher node and a runner node may share one realm
  and one placement topic without either knowing about the other.
- The one thing the dispatcher cannot do without is a **reader for the
  placement wire format** (the `soulstream-workloads/placement v1`
  marker + declaration JSON in the item body). Duplicating that reader
  would put the wire format in two places, so `fleet`'s existing
  unexported reader is **exported unchanged** as
  `fleet.DeclarationOf`. That is the whole of this feature's change to
  `fleet`: one identifier, no behavior.

### The `inference` block — HELD (design 0007 §3/§9 [O], amendment 2026-08-28)

Design 0007 §3 held its own schema at the operator's direction pending
research `inference-plane`. This feature therefore builds **nothing**
for inference: the declaration grows no `inference` field, the
dispatcher resolves no provider secret, and `wrap.Template.Env` is
untouched. Design 0007 §4's measured mechanics (wake-time resolve,
env injection, structural scope denials) stand unbuilt until that topic
answers. Acceptance bars 4 and 5 of design 0007 §8 are consequently
**not** in this feature's scope (see Out of Scope).

### Credentials arrive through a hook, never through this package (design 0007 §5 [V→O])

Per served placement the engine needs a `*realm.Client` bound to the
declared agent's persona. Which credential that is — a D28
`mint.ephemeral` against the deployment's persona-scope role, a creds
file on disk, an auth-callout token — is the **product's founding
ceremony**, not this repo's business (this repo has carried no
soulstream-identity dependency since the daemon cut, design 0004 §9).
The dispatcher takes a caller-supplied
`ConnectAgent func(ctx, persona) (*realm.Client, error)` and calls it
once per served placement. The `dispatcher serve` command ships the
one lane a node operator can wire with no new authority: a directory of
per-persona credential files.

### Drain and crash are different ends, chosen by the caller (design 0007 §6 [V])

The stop ceremony is **config, not chance**. Draining (engines
cancelled and waited for, so an in-flight harness returns a failure and
the engine posts its self-report) is an explicit act — `Drain`, which
`Run` performs when its context ends. Crashing (connections dropped,
nothing posted, the successor re-serves on the deterministic outcome
id) is **the process dying**: a supervisor that wants crash semantics
kills the process, and the dispatcher offers no in-process imitation of
it.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Submit and forget (Priority: P1)

A person submits an agent declaration to the placement topic and walks
away — closes their laptop, ends their session. A dispatcher node
racing that topic wins the placement, serves the declared agent, and
the agent answers a mention nobody's laptop was present for. The
dispatcher restarts; the record alone tells it what it already owns, so
it resumes with no new op and answers nothing twice. The dispatcher is
hard-killed mid-run; nothing was posted, and the restart serves that
same wake exactly once.

**Why this priority**: This is the feature — design 0007 §1's whole
loop and acceptance bar 1. Without it, "agents as infrastructure" is a
spike.

**Independent Test**: Hermetic realm; submit; close the submitter's
connection; run one dispatcher; mention the agent; assert one outcome.
Restart the dispatcher and assert the item's timeline gained no event
and the outcome count stayed 1. Kill (drop connections without
draining) mid-harness; assert zero outcomes; restart; assert exactly 1.

**Acceptance Scenarios**:

1. **Given** an open placement carrying an agent declaration with a
   wake set, **When** a dispatcher scans, **Then** it claims, reads the
   log back, and runs the wake engine for the declared persona — and a
   mention of that persona is answered exactly once under
   `WakeOpID(trigger, persona)`.
2. **Given** a placement this node already owns on the record,
   **When** a fresh dispatcher instance starts, **Then** it serves that
   placement with **no new op** — no second claim, no handshake, no
   local state consulted.
3. **Given** a wake in flight, **When** the dispatcher's connections
   drop without a drain, **Then** the topic gains no outcome, and
   **When** a dispatcher restarts, **Then** that wake is served exactly
   once.

---

### User Story 2 - Two nodes, one owner, and failover (Priority: P1)

Two dispatcher nodes watch one placement topic. Every contested
placement ends with one owner and one live claim on the record — the
log decided, no coordinator was asked. One node dies; the survivor's
sweep nominates the silent owner, the probe does not answer, and an
ordinary `work.abandon` reopens the item for a fresh race the survivor
wins. A mention posted inside the failover window is answered exactly
once, by the survivor. None of the probe traffic appears on the stream.

**Why this priority**: Design 0007 acceptance bar 2, and the whole
argument that a fleet needs no control plane.

**Independent Test**: Two dispatchers, several placements; assert one
owner and exactly one non-void claim per item; stop one node's probe
answers and connections; assert the survivor's timeline reads
`claim,abandon,claim`; post a mention in the window and assert one
outcome; scan the materialised topic for probe-shaped ops.

**Acceptance Scenarios**:

1. **Given** two dispatchers racing N placements, **When** the races
   settle, **Then** every item has exactly one non-void claim and its
   owner is the node serving it.
2. **Given** a placement owned by a node that dies, **When** the
   survivor sweeps past the reclaim bound, **Then** the item's timeline
   is `claim,abandon,claim`, the survivor owns it, and no `done` event
   was folded.
3. **Given** a mention posted while the placement is between owners,
   **When** the survivor serves it, **Then** the topic holds exactly
   one outcome for that trigger.
4. **Given** any of the above, **When** the placement topic is
   materialised, **Then** no probe subject or probe payload appears on
   it.

---

### User Story 3 - The colony budget rides the dispatcher path (Priority: P1)

Two declared agents that always mention each other are served by the
dispatcher. Their declared budget halts the cycle at its bound, op-less
and loud — the same 0006 admission that guards a laptop wrapper, because
the dispatcher serves through the same engine and adds no admission
point of its own. A legitimate owner→A→B→A delegation under the
defaults completes with zero refusals.

**Why this priority**: Design 0006 §6's standing rule — a dispatcher
inherits the budget requirement, and "inherits" must be *measured*, not
asserted.

**Independent Test**: Two agent declarations with `budget.max_hops`
submitted to one dispatcher; a person posts one turn; assert the agent
turn count equals the bound and at least one `wake_refused` line, with
no refusal op in the topic. Separately, the same shape under default
budgets with a terminating script: three outcomes, zero refusals.

**Acceptance Scenarios**:

1. **Given** two dispatcher-served agents whose replies always mention
   each other and a declared `max_hops`, **When** a person starts the
   cycle, **Then** it halts at exactly that many agent turns with ≥1
   loud refusal and no refusal testimony posted.
2. **Given** the same two agents under default budgets and scripts that
   terminate, **When** a person delegates, **Then** the chain completes
   with zero refusals.

---

### User Story 4 - The runner path is not disturbed (Priority: P2)

A placement whose declaration is not an engine-servable agent (a
`role: tool` declaration, or an agent with no wake set — a backend
workload the fleet's `Runner` path owns) is **left alone** by the
dispatcher: not claimed, not served, not abandoned. A dispatcher node
and a `fleet.Node` runner node can share one realm and one placement
topic.

**Why this priority**: Design 0003 §2's self-selection rule ("a node
MUST claim only work it can actually run") is what makes heterogeneous
fleets possible without new vocabulary. Breaking it would silently
strand backend workloads.

**Independent Test**: Submit one servable and one non-servable
placement to a dispatcher; assert the non-servable one stays open with
an empty timeline while the servable one is claimed and served.

**Acceptance Scenarios**:

1. **Given** a placement whose declaration has `role: tool`, **When**
   the dispatcher scans, **Then** the item is still open and its
   timeline is empty.
2. **Given** a placement whose declaration is `role: agent` with no
   `wake` entries, **When** the dispatcher scans, **Then** the same.

---

### Edge Cases

- **A won placement that cannot be served** (the `ConnectAgent` hook
  fails, or the declaration will not map onto an engine config) is
  handed straight back with an ordinary `work.abandon` and a loud log
  line — the same discipline `fleet.TryPlace` applies to a failed
  launch. A node-local backoff keeps a permanently-unservable
  declaration from becoming a claim/abandon spin on the record: the
  node simply does not re-race that item for a bounded pause. The
  backoff is transient, node-local, and delays a decision — it never
  makes one (design 0003 §1).
- **Two placements naming the same persona** would put two engines on
  one credential, both racing the same deterministic outcome ids. The
  dispatcher refuses the second loudly and does not claim it — ordinary
  self-selection, no new vocabulary.
- **A placement claimed by this node that is not engine-servable** is
  left running/owned by whatever claimed it; the dispatcher neither
  serves nor releases it.
- **A drain with retries configured**: `wrap`'s retry loop returns the
  context error rather than posting a self-report when the wake is
  cancelled between attempts, so the self-report testimony §6 measured
  is what a `Retries: 1` engine produces. Retries above 1 turn a drained
  wake into a parked wake — answered by the successor instead. Recorded,
  not changed: the choice belongs to whoever configures the engine.
- **The placement topic and an agent's home topic may be the same
  path**: every agent op then pokes the dispatcher's scan. Correct but
  chatty; the poll fallback makes it merely redundant, never wrong.
- **A live subscription that misses an op** (a slow consumer, a blip)
  costs nothing: the poll fallback re-scans on its cadence and the
  claim path is idempotent against the log.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A new `dispatcher` package MUST provide a standing
  per-node process that watches one placement topic, races open
  placements, and serves what it owns. It MUST hold no durable state:
  the record is the only position (constitution soulstream-workloads I).
- **FR-002**: The watch MUST be **live** — a plain subscription on the
  placement topic's ops subject (`topic.OpsSubject(path)`) — with a
  **materialise-based poll as catch-up only** (design 0007 §9, named a
  build requirement precisely so the spike's poll is not copied). Any
  op on the topic triggers one scan; the poll triggers one scan on its
  cadence; the two are the same scan and are idempotent against the log.
- **FR-003**: One scan MUST materialise the placement topic once and,
  from that single view: serve every placement whose status is
  `claimed`, whose owner is this node, and which this node is not yet
  serving (**resume**, with no op published); and attempt every open
  placement the node self-selects for (**race**).
- **FR-004**: The race MUST be the ordinary claim path: publish
  `ClaimWork`, re-materialise, and serve **only if** the read-back names
  this node as the item's owner. `fleet.Node.TryPlace` MUST NOT be
  called and `fleet` MUST NOT grow a launch hook (the resolved [O]
  above); `fleet`'s only change is the export of its placement-body
  reader.
- **FR-005**: A placement is **engine-servable** when its declaration
  has `role: agent` and at least one `wake` entry. Everything else MUST
  be left alone — not claimed, not served, not abandoned — so the
  fleet's `Runner` path keeps owning it.
- **FR-006**: Serving MUST be `wrap.DeclaredConfig` over a caller-
  supplied base config plus a `*realm.Client` obtained from the
  `ConnectAgent(ctx, persona)` hook, run as a `wrap.Wrapper`. The
  dispatcher MUST NOT resolve, hold, or inject any credential itself,
  and MUST NOT add an admission point of its own — the 0006 budget is
  the engine's, unchanged.
- **FR-007**: A base engine config carrying a fixed `Persona` MUST be
  refused at startup with a teaching error: one dispatcher serves
  whatever personas its placements declare.
- **FR-008**: A placement won but not servable (hook error, config
  mapping error, engine start error) MUST be abandoned back onto the
  record with a loud log line, and MUST NOT be re-raced by this node
  for a bounded, node-local pause.
- **FR-009**: The dispatcher MUST answer `fleet.ProbeSubject(<node>)`
  for exactly the placements it currently serves, with `fleet`'s wire
  answers (`alive` / `no`), so a peer's probe vetoes a live owner. The
  owned set MUST be safe for concurrent access — the subscription
  callback and the serve/drain paths run on different goroutines.
- **FR-010**: The dispatcher MUST run `fleet.Node.Sweep` on a cadence
  with a `fleet.Node` carrying only `ID`, `Conn`, and the two bounds
  (no `Runner`), so a dead peer's placements reclaim as an ordinary
  `work.abandon` and reopen for a fresh race. Probe and sweep traffic
  MUST stay off the stream.
- **FR-011**: `Drain(ctx)` MUST stop claiming, cancel every engine, and
  **wait** for them, so an in-flight harness failure lands its
  self-report before the process ends. `Run` MUST drain when its context
  ends, and `Drain` MUST be idempotent. No in-process "hard stop" is
  offered: crash semantics are the process dying.
- **FR-012**: `soulstream-workloads dispatcher serve` MUST run one
  dispatcher from node-side configuration only (constitution III — none
  of it may appear in a declaration) and MUST drain on SIGINT/SIGTERM.
  Its `ConnectAgent` lane MUST be a directory of per-persona credential
  files; a missing file refuses that placement loudly (and the placement
  returns to the race), never half-serves.
- **FR-013**: The dispatcher MUST NOT publish, log, or otherwise emit
  the contents of any credential; the served agent's authority is its
  own connection, exactly as in `wrap` (design 0005's runtime-side-reads
  standing is inherited unchanged).

### Key Entities

- **Placement**: an ordinary work item on the placement topic whose
  body is the `soulstream-workloads/placement v1` marker plus the
  declaration JSON. Submission is `fleet.Submit`; nothing else is new.
- **Scan**: one materialise of the placement topic plus the resume and
  race passes derived from it. Triggered live by an op or by the poll.
- **Serve**: one running `wrap.Wrapper` for one owned placement — its
  cancel function, its done channel, and the persona it is bound to.
- **Owned set**: the placements this node is serving right now; the
  answer to a peer's liveness probe and nothing else.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With the submitter's connection closed, a mention of a
  dispatcher-served agent is answered exactly once (design 0007 §8.1).
- **SC-002**: A dispatcher restart resumes every placement the log says
  it owns and publishes **no** op doing so; the answered wake's outcome
  count stays 1 and no harness is re-invoked for it.
- **SC-003**: Connections dropped mid-run leave **zero** outcomes; a
  restart then serves that wake exactly once (design 0007 §6's crash
  arm).
- **SC-004**: Across two nodes and repeated contested placements, every
  item ends with exactly one owner and exactly one non-void claim
  (design 0007 §8.2).
- **SC-005**: A killed node's placement reclaims within the configured
  bound as `claim,abandon,claim` from the survivor, and a mention posted
  in the failover window is answered exactly once.
- **SC-006**: No probe subject, probe payload, or other transient
  signalling appears anywhere in the placement topic's materialised view.
- **SC-007**: A declared budget halts the uncooperative two-agent cycle
  at its bound through the dispatcher path, with ≥1 loud `wake_refused`
  and zero refusal ops; the legitimate delegation completes with zero
  refusals under defaults (design 0007 §8.3).
- **SC-008**: A non-servable placement (role tool, or an agent with no
  wake set) is untouched by a running dispatcher: still open, empty
  timeline.
- **SC-009**: The pre-011 suites pass unmodified; `fleet`'s behavior is
  byte-for-byte today's (its only diff is one exported identifier).

## Out of Scope (named follow-ons)

- **The `inference` block and provider-secret custody** (design 0007
  §3–§4): held at the operator's direction pending research
  `inference-plane`. Design 0007 acceptance bars 4 and 5 belong to
  whatever feature answers it.
- **The founding's role naming and engine-credential TTL/renewal**
  (design 0007 §5 [O]) — the product's spec, reached here only through
  the `ConnectAgent` hook.
- **The shell's declare surface** (design 0007 §7) — a pure class-(a)
  shell module, designed at its own build.
- **The grants-broker lane** for agents thinking on a *person's*
  provider account (design 0007 §4 [O]) — outbound identity, its own
  demand gate.
- **A serve hook on `fleet.Node`** (design 0007 §2 option (a)) — the
  road not taken; recorded so it is not re-opened without a new reason.
- **Zombie cap** (design 0003 §3 [O]): an owner answering probes
  without progressing still suppresses reclaim indefinitely. Unchanged
  by this feature, still open.

## Assumptions

- The placement topic exists and the node's credential may read its ops
  subject and publish claims/abandons on it — the node scope of design
  0003 §4, node-side configuration.
- `ConnectAgent` returns a client whose persona **is** the declared
  persona; `wrap.DeclaredConfig` refuses the mismatch, and that refusal
  is the check (the connection's admission is the authority — design
  0005's standing).
- Reclaim bounds are node-side configuration and the probe timeout fits
  inside the reclaim bound (design 0003 §6).
- Clock skew and clustered JetStream carry design 0003 §7's open
  limits unchanged — this feature adds no new dependence on either.
- The engine's exactly-once guarantee (outcome existence under the
  deterministic wake id) is the only dedupe the dispatcher needs; it
  adds none of its own.

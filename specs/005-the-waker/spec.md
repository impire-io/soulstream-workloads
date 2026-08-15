# Feature Specification: The waker — notify-triggered invocation

**Feature Branch**: `005-the-waker`
**Created**: 2026-08-15
**Status**: Draft
**Input**: Roadmap Phase 3 M3.2, from design
[`soul-hq/02-DESIGN/soulstream-workloads/0004-the-waker.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0004-the-waker.md)
(graduated from the `agent-participation` research topic — all four
pre-registered bars measured PASS, episode
[0082](../../../soul-hq/04-JOURNEY/0082-ecosystem-agent-participation.md)).

The waker is the workload plane's **trigger arm**: a standing component that
turns a mention of a registered agent into one invocation of a headless
harness, and guarantees the topic exactly one outcome per admitted wake. It
is what makes an agent addressable like a person whether or not a process
exists — the missing third of agent participation (identity and acting
landed in episode 0079; waking is this feature). Nothing here adds wire
vocabulary: the waker is a consumer and a client of the record, never a
second control plane (constitution I).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A mention wakes an agent that isn't running (Priority: P1)

An operator registers an agent with the waker: its persona, its credential
reference, and its invocation template (how to run one turn of its harness).
No agent process runs. Someone @-mentions the agent in a topic. The waker —
the only resident process — wakes the harness headlessly with the
conversation's context; the harness's final answer lands in the topic as
exactly one turn authored by the agent's persona.

**Why this priority**: This is the feature's entire point — the wake path.
Without it agents are only *runnable* (M1.1); with it they are *addressable*.

**Independent Test**: Register a scripted harness as an agent, post a
mention while no agent process exists, assert exactly one reply turn
authored by the agent appears.

**Acceptance Scenarios**:

1. **Given** a registered agent and no running agent process, **When** a
   persona posts a turn @-mentioning the agent, **Then** exactly one reply
   turn authored by the agent's persona appears in the same topic, produced
   by a harness process the waker started and which exited afterwards.
2. **Given** the same setup with the harness's posting tool removed from
   its tool surface, **When** the mention is posted, **Then** the reply
   still lands — the answer must not depend on the model choosing to call
   a tool (the reply obligation is the waker's, measured in research).

---

### User Story 2 - The conversation always learns the outcome (Priority: P2)

A harness crashes mid-run, hangs past its time budget, or answers by
posting into the topic itself mid-run and ends with only a report about
having done so. In every case the topic ends up with exactly one outcome
for that wake: the reply, or — after the retry budget is spent — a failure
turn spoken by the waker's own persona, naming the agent and the asker.
Nothing dangles, nothing duplicates.

**Why this priority**: Request/reply with a completion guarantee is what
makes agents composable in the stream; a wake that can silently vanish or
double-post makes mentions untrustworthy.

**Independent Test**: Inject each fault (kill, hang-to-budget, mid-run
self-post) against a registered agent; count outcome ops per wake — each
must be exactly one, and consumer state must show nothing unacknowledged.

**Acceptance Scenarios**:

1. **Given** a harness that dies mid-run on every attempt, **When** the
   retry budget is spent, **Then** exactly one failure turn appears,
   authored by the waker's persona, naming the agent, the asker, and the
   reason.
2. **Given** a harness that exceeds the run time budget on every attempt,
   **When** the budget is spent, **Then** the same single-failure-turn
   outcome holds.
3. **Given** a harness that posts its reply into the topic mid-run,
   **When** the run ends with a terminal message that is only a report
   about having replied, **Then** the mid-run turn stands alone — the
   waker posts nothing and acknowledges the wake.
4. **Given** any completed set of trials, **When** the notify consumer is
   inspected, **Then** no deliveries remain unprocessed or pending.

---

### User Story 3 - The address outlives the process, and revocation bites the wake (Priority: P3)

Mentions posted while the waker (or its host) is down accumulate; when the
waker returns, every one of them is answered, in order. When the operator
takes the agent's credential away, the next mention produces **no** wake —
no harness run, no op — while the persona stays mentionable and its history
stays attributed; when the operator re-grants the credential, the *same*
pending mention gets its answer. Revocation is a delay for the asker, never
a loss.

**Why this priority**: This is what separates an addressable agent from a
process with a subscription — and the revocation semantics are the
operator's stop button, proven in research to bind at the very next wake.

**Independent Test**: Accumulate three mentions with the waker down, start
it, assert three replies. Revoke, mention, assert no op and a still-visible
attributed history; re-grant, assert the pending mention is answered.

**Acceptance Scenarios**:

1. **Given** three mentions posted while no waker runs, **When** the waker
   starts, **Then** all three receive their replies and none is lost.
2. **Given** a revoked agent, **When** it is mentioned, **Then** no op of
   any kind is authored by the agent, the mention remains pending, the
   persona remains mentionable, and prior turns remain attributed to it.
3. **Given** the credential is re-granted, **When** the pending mention is
   redelivered, **Then** it receives its reply — the same mention, not a
   new one.

---

### User Story 4 - A new harness is configuration, not code (Priority: P4)

An operator onboards a second, structurally different harness — a different
command line, a different event grammar — by writing an invocation template
alone. The waker binary does not change.

**Why this priority**: The research's sharpest economic result: per-harness
cost collapsed to a template. If onboarding a harness needs waker code, the
design's premise (harnesses are configuration) has failed — this story is
the regression guard on that premise.

**Independent Test**: Run User Story 1's flow against two harnesses whose
event grammars differ structurally, with the waker binary byte-identical
and only the template swapped.

**Acceptance Scenarios**:

1. **Given** two registered agents whose templates name different commands
   and different terminal-event mappings, **When** each is mentioned,
   **Then** each replies through its own harness, and the waker binary is
   the same bytes for both.

---

### Edge Cases

- **Several mentions in one topic** (the measured trap): an earlier wake's
  reply must never masquerade as a later mention's answer — outcome
  correlation must compare what the run itself produced, not stream order.
- **Redelivery after an outcome but before its acknowledgement** (waker
  dies in the window): the outcome post must be idempotent across
  redeliveries so the topic never carries a duplicate reply.
- **A harness with no machine-readable terminal event**: refused at
  registration time — a template must name its terminal mapping or the
  agent cannot be registered (degraded wrapping is a deliberate operator
  act, named as such).
- **Notify messages that are not mentions**: the notify subject is
  deliberately general; non-mention types are acknowledged without a wake
  and without an op, and are visible in the waker's log.
- **A mention in a topic the agent cannot read**: context materialization
  fails; the wake follows the failure path (retry budget, then waker-voiced
  failure turn).
- **The waker's own restart mid-run**: the in-flight delivery redelivers
  after its acknowledgement window; at-least-once delivery plus idempotent
  outcomes keep the invariant.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The waker MUST be a standing component that serves any number
  of registered agents in one realm, holding one durable notify
  subscription per agent; mentions MUST accumulate while the waker is down
  and MUST be processed in stream order when it returns.
- **FR-002**: A wake MUST be acknowledged only after its outcome is
  decided: an outcome op exists in the topic (admitted wake), a mid-run
  post was correlated (admitted wake), or redelivery is scheduled (refused
  or retried wake). The retry budget MUST be waker policy; the transport
  MUST never silently discard a notify message.
- **FR-003**: Before spending a harness run, the waker MUST probe the
  agent's admission by connecting as the agent; a refused probe MUST
  refuse the wake — no harness run, no op of any kind — leaving the
  mention pending for redelivery.
- **FR-004**: The invocation template MUST be pure configuration — command
  with placeholders, machine-readable terminal-event mapping, environment,
  run time budget, retry budget — and the waker MUST contain no
  harness-specific code. A registration whose template names no terminal
  mapping MUST be refused.
- **FR-005**: The waker MUST own the reply obligation: the harness's
  terminal text is posted as the agent's turn; a turn the harness posted
  itself during the run MUST be detected by comparing the topic before and
  after the run (never by stream-order anchoring — measured failure) and
  MUST NOT be duplicated. Outcome posts MUST be idempotent across
  redeliveries of the same wake.
- **FR-006**: A wake that fails at its retry budget MUST produce exactly
  one failure turn authored by the **waker's own persona** — never the
  agent's voice — naming the agent, the asker, and the legible reason
  (design 0004 §7; forced by the measured revocation semantics).
- **FR-007**: The wake credential MUST follow one of two lanes, per
  registration: per-wake connections on the agent's revocable registration
  (revocation binds at the next wake), or per-run ephemeral credentials the
  waker mints with lifetime bounded by the run budget. In the ephemeral
  lane nothing longer-lived than the run reaches harness configuration.
  Minting is the waker's act; an agent MUST NOT be able to mint for
  itself.
- **FR-008**: Each run MUST get a fresh working directory and a sanitized
  environment (no realm configuration inherited from the waker's host
  environment); a run past its time budget MUST be terminated with its
  whole process tree.
- **FR-009**: Harness narration (mid-run messages before the terminal
  event) MUST NOT become topic ops. (Relaying narration as ephemeral
  presence is permitted by design 0004 §9 and out of scope for this
  feature.)
- **FR-010**: The waker MUST keep the substrate boundary: it stores nothing
  worth keeping outside the record — run directories are scratch,
  registrations live in configuration and the identity plane, and every
  durable fact it produces is an op in a topic (constitution I).
- **FR-011**: The default quality gate MUST stay hermetic: no external
  harness, no external server. Default tests MUST prove the wake protocol
  against scripted harnesses; a real-harness proof MAY be an opt-in target
  (the M1.3/M2.1 pattern).
- **FR-012**: Lifecycle visibility MUST NOT regress: the waker publishes
  outcome ops and nothing else — no second control plane, no
  waker-specific subjects on the record's stream (constitution I, V).

### Key Entities

- **Waker** — the standing trigger arm: one process, one realm, many
  registered agents; support-layer standing (its consumer and minting
  rights exceed any workload scope by design — measured in research).
- **Registration** — one agent as the waker knows it: persona, credential
  lane and reference, invocation template. The revocable credential
  registration in the identity plane is the fact the admission probe
  checks.
- **Invocation template** — the per-harness configuration: command argv
  with placeholders, terminal-event mapping (dot-path type field, terminal
  value, text field, optional status/success), environment block, run and
  retry budgets.
- **Wake** — one delivery of one notify message: ends admitted (exactly one
  outcome op) or refused/retried (no op, redelivery pending).
- **Outcome op** — the one turn an admitted wake leaves: the agent's reply
  (agent-authored) or the waker's failure testimony (waker-authored).
- **Admission probe** — a connection attempt as the agent itself, made
  before each harness run; the revocation enforcement point.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A mention of a registered agent posted while no agent
  process exists yields exactly one reply turn authored by the agent —
  including when the harness's posting tool is removed from its surface.
- **SC-002**: Kill, hang-to-budget, and mid-run self-post trials each end
  with exactly one outcome op per wake (waker-authored failure for the
  first two, the mid-run turn alone for the third), and the notify
  consumer ends every trial with nothing unprocessed or pending.
- **SC-003**: Three mentions accumulated while the waker is down all
  receive replies after it starts; after revocation the next mention
  produces no op while the persona stays mentionable and its history
  attributed; after re-grant the same pending mention is answered.
- **SC-004**: A second, structurally different harness passes SC-001 with
  the waker binary byte-identical — the diff between the two agents is
  their templates alone.
- **SC-005**: The full quality gate (`make fmt && make test && make lint`)
  is green with the default gate hermetic; the wake protocol's proofs run
  against scripted harnesses inside it.

## Assumptions

- **Registration source**: day one, registrations are waker configuration
  (file or flags). Surfacing them from the shell's Agents module (episode
  0079) is a product-composition follow-up, not this feature.
- **The waker's own persona** exists as an ordinary operated persona
  (episode 0079's ceremony applies to it as to any agent); how the product
  composes/founds it is out of scope here — the waker takes its persona
  and credential as configuration.
- **Identity plane reachable** in deployments using the admission probe or
  the ephemeral lane; a rig without it (open server) exercises the wake
  protocol with the probe disabled, as research did for bars 1/2/4.
- **Loop safety** (agent-wakes-agent budgets) is deliberately out of scope
  — named `[O]` in design 0004 §10 with its own successor research topic
  pre-registered before any such deployment.
- **Presence relay** of harness narration is deferred (design 0004 §9);
  this feature only guarantees narration never becomes ops.
- **Fleet interplay**: this feature ships the single-node waker; how a
  fleet claims wakes (design 0003's claim path) is specified when fleet
  placement is built (M3.1), against the same declaration vocabulary.

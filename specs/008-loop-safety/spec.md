# Feature Specification: Loop safety — the wake budget

**Feature Branch**: `008-loop-safety`
**Created**: 2026-08-25
**Status**: Draft
**Input**: soul-hq design [`0006-loop-safety.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0006-loop-safety.md), graduated from research episode 0128 (all four bars measured). A composed wake budget enforced at wake admission in the wrapper: an authorship-window floor and a provable-chain depth bound, both computed from the topic view the wake already reads; refusals op-less and loud; zero budget reproduces today's behavior.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The runaway cycle halts without cooperation (Priority: P1)

An operator wraps two (or more) agents that mention each other in a shared topic. One of them misbehaves — by prompt injection, model error, or plain bad instructions — and every reply mentions another agent, forever. Today this burns harness runs at the measured rate of ~84 wakes per second per pair (episode 0128); with the budget, the room stops the cascade at a bound the operator can compute in advance, without any agent choosing to stop.

**Why this priority**: This is the reason the feature exists — the colony gate. Without it, agent-wakes-agent deployments are barred (design 0005 §7).

**Independent Test**: Two wrapped agents with scripts that always mention each other, one human root mention; the cascade must halt at exactly the depth bound with a loud, op-less refusal.

**Acceptance Scenarios**:

1. **Given** two wrapped agents whose replies always mention each other and a depth bound D, **When** a person posts one mention, **Then** exactly D agent outcomes land, the next wake is refused with one structured `wake_refused` log line carrying the numbers, and no refusal op of any kind appears in the topic.
2. **Given** the same cycle where both agents post their outcomes through their own client under arbitrary op ids (the MCP arm — outcomes invisible to the chain walk), **When** the cascade runs, **Then** the window floor halts it within 2K outcomes (K = window max per agent), refusals loud and op-less.

---

### User Story 2 - Legitimate delegation is untouched (Priority: P1)

A person asks agent A for something; A asks agent B; B answers back to A; A concludes. Nothing about the budget may break this — the mechanism is a budget, not a ban.

**Why this priority**: A halting mechanism that also halts legitimate work is the blunt alternative the research explicitly discriminated against (Bar 3).

**Independent Test**: A human-rooted A→B→A chain shorter than the depth bound completes under default budgets with zero refusals.

**Acceptance Scenarios**:

1. **Given** default budgets and a human-rooted delegation owner→A→B→A, **When** the chain runs, **Then** exactly 3 agent outcomes land, zero refusals fire, and every outcome is attributable to the human root through the chain walk.
2. **Given** an ordinary single mention of one wrapped agent by a person, **When** the agent answers, **Then** behavior is indistinguishable from today's.

---

### User Story 3 - The operator can see why (Priority: P2)

When a wake is refused — or an operator wonders where a burst of agent turns came from — the answer must be readable: the refusal log line carries the numbers (hops vs bound, window count vs max), and the ancestry walk (which op provably triggered which outcome, chained to the root) is available as a library surface for diagnostics.

**Why this priority**: Loud refusal is a design invariant (0083 precedent); the walker is what makes a refusal explainable and a cascade auditable.

**Independent Test**: Unit-level — the walker resolves provable chains, reports depth, and reports (never absorbs) ambiguity; the refusal log line contains persona, trigger op, and the legible reason with numbers.

**Acceptance Scenarios**:

1. **Given** a topic containing wrapper-posted outcome chains, **When** the walker runs over the view, **Then** every outcome resolves to its trigger and its root, with depth, and an op with two parent candidates is reported as ambiguous rather than silently picked.
2. **Given** a refused wake, **When** the operator reads the log, **Then** one `wake_refused` line states which budget refused and the measured numbers that crossed it.

---

### User Story 4 - Opting out is explicit and byte-identical (Priority: P3)

An operator who sets both budget knobs to zero gets exactly today's wrapper: no gate, no behavior change, and one startup log line stating the unbudgeted standing.

**Why this priority**: Compatibility floor; makes the default-on budget an honest choice rather than a silent change.

**Independent Test**: Zero-budget config run through the existing wake tests — identical outcomes, plus the one startup line.

**Acceptance Scenarios**:

1. **Given** MaxHops=0 and WindowMax=0, **When** any wake arrives, **Then** the gate never runs, outcomes match today's behavior exactly, and startup logged the unbudgeted standing once.

---

### Edge Cases

- A refusal must never become an outcome: no self-report, no failure turn, no op — the measured harness-slot variant (312-turn failure ping-pong) is the anti-pattern the placement rule exists to prevent.
- A refused wake is a delay, not a loss: the mention stays in the bounded inbox; a later catch-up re-evaluates the gate (a slid window may then admit), and the deterministic outcome id still guarantees at most one outcome.
- The trigger op may be absent from the view (rolled up): the depth walk treats it as a root (0 provable hops); the window floor still applies.
- Concurrent mentions in one topic: chain resolution binds by outcome id, never stream order (the 0082 correlation lesson); budgets are evaluated per wake against the view read for that wake.
- An agent's own self-mention still never wakes it (shipped guard, unchanged, evaluated before the budget).
- Ambiguous parentage (two candidate triggers for one outcome id) is practically impossible (UUIDv5) but must surface as a reported condition, not a silent choice.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The wrapper MUST evaluate a wake budget at wake admission — after the self-skip and the outcome-existence pre-check, before any harness invocation — using only the topic view already read for the wake.
- **FR-002**: The window floor MUST refuse a wake when the wrapped persona already authored WindowMax turn.post contributions in the wake's topic within WindowPer, counted from the view's timestamps.
- **FR-003**: The depth bound MUST refuse a wake when the outcome would sit more than MaxHops provable hops from a chain root, where one hop is the deterministic outcome-id binding (UUIDv5 of trigger op id and persona) verified against candidate ops in the view — never inferred from stream order.
- **FR-004**: A refusal MUST post nothing — no outcome, no self-report, no op of any kind — and MUST emit exactly one structured `wake_refused` log entry naming the persona, topic, trigger op, which budget refused, and the numbers (hops vs MaxHops, or window count vs WindowMax/WindowPer).
- **FR-005**: A refusal MUST NOT acknowledge the wake as answered: the mention remains re-presentable by catch-up, and a later evaluation with a slid window MAY admit it; the deterministic outcome id keeps at-most-one outcome regardless.
- **FR-006**: The ancestry walker MUST ship as a pure, I/O-free surface: provable parent of an op (with match count), chain to root, and depth; more than one parent match MUST be reported to the caller, never silently resolved.
- **FR-007**: The budget MUST be configuration on the wrapper (MaxHops, WindowMax, WindowPer), applied with defaults MaxHops=4, WindowMax=8, WindowPer=10m; a knob set to zero disables that part.
- **FR-008**: With both knobs zero the wrapper MUST behave byte-identically to today (no gate evaluation on any wake) and MUST log the unbudgeted standing exactly once at startup.
- **FR-009**: The self-skip guard and the outcome-existence pre-check MUST remain evaluated before the budget, unchanged.
- **FR-010**: The budget MUST require no new op types, no core wire change, and no state held anywhere but the record and the wrapper's configuration.

### Key Entities

- **Wake budget**: the pair (depth bound, window floor) with its three knobs; part of the wrapper's configuration.
- **Provable hop**: the verifiable binding between an outcome op and its trigger op via the deterministic outcome id; the unit the depth bound counts.
- **Chain root**: an op with no provable parent in the view — a human post, a rolled-up ancestor, or an outcome posted under an arbitrary id.
- **Refusal**: the op-less, logged decision not to admit a wake; carries its reason and numbers; never terminal for the mention.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An uncooperative two-agent mention cycle halts at exactly MaxHops agent outcomes from one human root (research baseline without the budget: 421 outcomes in 5 seconds and still growing).
- **SC-002**: The same cycle with id-evading (self-posted) outcomes halts within 2×WindowMax outcomes (research baseline without the window: 393 outcomes in 3 seconds past an evaded depth gate).
- **SC-003**: A human-rooted delegation chain of 3 outcomes completes with zero refusals under default budgets.
- **SC-004**: Zero refusal ever appears as a contribution in any topic, in any test, under any budget setting.
- **SC-005**: With both knobs at zero, the full existing wake test suite passes unchanged.
- **SC-006**: Every refusal is explainable from its single log entry alone (budget kind + numbers), and every wrapper-posted outcome in a test topic is attributable to its root through the walker with zero ambiguity.

## Assumptions

- Defaults (MaxHops=4, WindowMax=8, WindowPer=10m) come from design 0006 §3 [judgment]: generous against every legitimate flow measured, orders of magnitude under the danger numbers (84 wakes/s pair cycle, 1,264.7 ops/s colony).
- The budget lands in the wrap package now; the same gate applies at the admission seam of any future wake dispatcher (design 0005), which is out of scope here.
- The window floor counts turn.post contributions only (outcomes and self-reports are turn.posts; presence and comments are not wakes' outcomes).
- CLI flag plumbing for `soulstream wrap` (the product binary) follows the config surface but its product wiring lives with the wrap-in-the-house composition, out of scope beyond the library Config.
- Timestamps in the materialised view are server-stamped stream time, trusted as the window clock.

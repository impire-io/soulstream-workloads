# Feature Specification: Wrap — run your agent where you are

**Feature Branch**: `006-wrap`
**Created**: 2026-08-15
**Status**: Draft
**Input**: The operator's direction on landing day of specs/005: "waker" is
not a good name, and the easiest attach path is personal — "I can run one
on my computer and have it wrapped by `soulstream wrap`." Design
[`0004-wrap.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0004-wrap.md)
(reshaped from the waker; the central daemon cut with its reversal
recorded).

The wrap engine keeps everything specs/005 measured — templates, harness
execution, snapshot correlation, deterministic outcome ids, the loop
guards — and repackages it as **one process, one agent, one credential**:
the wrapper runs on the machine where the person's assistant already
lives (their logins, their config), holds only the agent's own credential
block, and needs no consumer state because *the record is the position*.
`soulstream wrap` reaches it through core's new external-subcommand seam
(core specs/019, v0.8.4).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Wrap the assistant you already run (Priority: P1)

A person creates an agent in the shell (episode 0079), sets the block's
environment on their own machine, and runs `soulstream wrap --harness
claude`. Mentions of the agent — including ones posted while the wrapper
was not running — each get exactly one reply authored by the agent,
produced by their own logged-in assistant.

**Why this priority**: This is the feature: attaching an agent becomes one
command on hardware the person already trusts, and the provider-login
problem dissolves because the harness runs where the login lives.

**Independent Test**: With a scripted harness preset, post a mention with
no wrapper running; start the wrapper; assert exactly one reply under the
deterministic wake id.

**Acceptance Scenarios**:

1. **Given** the agent's credential environment and a harness preset,
   **When** the wrapper starts after a mention was posted, **Then**
   exactly one reply turn authored by the agent appears, under the wake's
   deterministic op id.
2. **Given** a running wrapper, **When** a new mention arrives live,
   **Then** it is answered the same way, without restart.
3. **Given** answered mentions, **When** the wrapper restarts, **Then**
   nothing is answered twice.

---

### User Story 2 - The conversation still always learns the outcome (Priority: P2)

Faults keep the specs/005 discipline in one-persona form: a harness that
dies or hangs exhausts the wrapper's in-process retries and the agent
posts a **self-report** ("I was asked and could not answer: …", tapping
only the asker); a harness that posted its own reply mid-run is
correlated and never duplicated.

**Acceptance Scenarios**:

1. **Given** a harness that dies every attempt, **When** retries are
   spent, **Then** exactly one agent-authored self-report appears, under
   the wake id, tapping the asker and never the agent.
2. **Given** a hang past the run budget, **Then** the same single
   self-report invariant holds.
3. **Given** a mid-run self-post, **Then** the topic holds exactly one
   turn for that wake.

---

### User Story 3 - Revocation still bites, presets still generalize (Priority: P3)

Taking the credential away refuses the wrapper's connection (loudly, in
its log — no op, the agent cannot speak); the persona stays mentionable
and re-grant plus restart answers what accumulated inside the inbox
window. A structurally different harness is a preset or template change;
the engine is byte-identical.

**Acceptance Scenarios**:

1. **Given** invalid credentials against an enforcing server, **When**
   the wrapper starts, **Then** it exits with a loud refusal and posts
   nothing.
2. **Given** the codex-grammar preset instead of the claude one, **When**
   a mention arrives, **Then** the reply flows through the same engine
   with only configuration changed.

---

### Edge Cases

- **Self-mentions never wake** (the measured loop guard stays).
- **Non-mention notify types** are ignored with a log line.
- **The inbox window bounds catch-up** (newest 100 — protocol): older
  mentions fall away; stated, not hidden.
- **Reconnects re-run catch-up** — a network blip cannot lose a wake that
  is still in the window.
- **One wake at a time**: wakes run sequentially; a laptop is not a
  fleet.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The wrapper MUST hold only the agent's own credential (the
  0079 block's environment, or a context/creds file) — no operator
  credentials, no consumer creation, no minting.
- **FR-002**: Position MUST be the record: catch-up reads the persona's
  inbox and wakes exactly the mentions whose deterministic outcome op
  (UUIDv5 of notify op + persona) is absent; live arrivals run the same
  existence check. No JetStream consumer state anywhere.
- **FR-003**: Every outcome posts idempotently under the wake op id; a
  restart answers nothing twice; mid-run self-posts are correlated by
  before/after snapshot difference, never stream order.
- **FR-004**: Failures at the retry budget are the agent's **self-report**
  — its own voice, naming the reason, tapping only the asker. A mention
  authored by the wrapped persona never wakes it.
- **FR-005**: Harnesses are configuration: named presets (claude, codex)
  and custom template files share one schema — command placeholders,
  required terminal mapping, optional `env` block for the harness's own
  provider credential, optional MCP block; presets derive the MCP
  environment from the wrapper's own lane env so the common case needs no
  authoring.
- **FR-006**: Run hygiene keeps specs/005's measured shape: fresh run dir
  under scratch, `SOULSTREAM_*`-scrubbed child env (template `env` then
  applied on top), stdin closed, process-group kill at the budget, the
  typed event stream deciding the outcome.
- **FR-007**: The `waker serve` daemon, its multi-agent configuration,
  its durable consumers, its dialer lanes, and its second persona are
  **removed**; specs/005 stays frozen as the record of what they proved.
- **FR-008**: The default gate stays hermetic (scripted harnesses); the
  real-harness proof moves to `make test-wrap`.

### Key Entities

- **Wrapper** — one process, one agent, one connection; catch-up plus
  live subscription; sequential wakes.
- **Preset** — a named built-in template (claude, codex); a template file
  overrides.
- **Wake / outcome op / template** — as design 0004 §1/§5/§6.

## Success Criteria *(mandatory)*

- **SC-001**: Backlog and live mentions each yield exactly one attributed
  reply under the wake id; restart yields zero duplicates.
- **SC-002**: Die, hang, and self-post trials end with exactly one op per
  wake, the failure ones agent-authored self-reports tapping the asker
  only.
- **SC-003**: An enforcing server refuses a bad credential loudly with
  zero ops; the wrapped persona stays mentionable.
- **SC-004**: The codex-grammar preset passes SC-001 with the engine
  byte-identical.
- **SC-005**: `make fmt && make test && make lint` green, hermetic;
  `make test-wrap` wakes a real installed assistant.

## Assumptions

- Core stays at the published v0.8.3 (`PostTurnIdempotent`,
  `FetchInbox`); the v0.8.4 dispatch is core-CLI-internal and not a
  workloads dependency.
- The fleet-era serve arm returns per design 0004 §9's reversal
  condition; the ephemeral mint lane returns with it (a personal wrapper
  cannot mint — the op is operator-gated by design).
- Shell surfacing of the wrap one-liner ships separately.

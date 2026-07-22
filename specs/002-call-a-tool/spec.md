# Feature Specification: Call a tool — a discoverable capability

**Feature Branch**: `002-call-a-tool`
**Created**: 2026-07-22
**Status**: Draft
**Input**: Roadmap Phase 1 M1.2, from design [`hq/02-DESIGN/0001-soulrealm-runtime.md`](../../hq/02-DESIGN/0001-soulrealm-runtime.md) §3.

Soulrealm launches a **tool** workload — a persistent capability other workloads
call over the realm transport — and an **agent** discovers and calls it, under
the same one-identity model as M1.1. This adds the second role (`tool`) and the
machinery a *persistent service* needs: it does not post-and-exit like the M1.1
agent, so the runner learns to **launch and later stop** a workload, not only
run one to completion.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - An agent calls a tool and gets a result (Priority: P1)

An operator declares a tool (role `tool`, lifecycle `service`) — say an
uppercasing capability. Soulrealm launches it with a scoped credential; it
serves request-reply on the realm transport. An agent, given the tool's name,
**discovers** it and sends a request; the tool replies; the agent uses the
result. Both are ordinary personas with scoped credentials — no privileged tier.

**Why this priority**: The MVP for M1.2 — it proves the `tool` role, a
persistent service lifecycle, discovery, and a scoped request-reply call end to
end. Nothing smaller demonstrates a tool exists.

**Independent Test**: Launch the uppercase tool, then run an agent that
discovers it by name and calls it with `"hi"`; assert it receives `"HI"`.

**Acceptance Scenarios**:

1. **Given** a launched tool service, **When** an agent discovers it by name and
   sends a request, **Then** the agent receives the tool's reply.
2. **Given** the tool and agent running, **When** each acts, **Then** the tool's
   credential permits serving only its service subjects and the agent's permits
   calling — neither can act outside its scope.
3. **Given** a request naming an unknown tool, **When** the agent tries to
   discover it, **Then** discovery returns nothing (no hang, no error tier).

---

### User Story 2 - A tool's run is legible and bounded (Priority: P2)

The tool is a *service*: it runs until stopped, not until it exits on its own.
Its life is still legible on the topic — launched (open + claim) and, when
soulrealm stops it, done — and stopping it reaps its process, scratch, and
credential.

**Why this priority**: Constitution V for a persistent workload, and the runner
capability (launch/stop) that M1.1's run-to-completion model lacked.

**Independent Test**: Launch the tool, confirm `work.open` + `work.claim` on the
topic; stop it; confirm `work.done` and that the process/scratch/creds are gone.

**Acceptance Scenarios**:

1. **Given** a tool declaration, **When** soulrealm launches it, **Then**
   `work.open` and `work.claim` appear and the process stays up.
2. **Given** a running tool, **When** soulrealm stops it, **Then** `work.done`
   appears and its process, scratch dir, and creds are reaped.

### Edge Cases

- **Tool never becomes ready** — launch has a readiness bound; if the service
  does not register within it, soulrealm stops it and `work.abandon`s.
- **Agent calls before the tool is up** — discovery finds nothing; the agent
  retries or gives up; no partial state.
- **Tool crashes while serving** — `work.abandon(nonzero-exit|signal)`, same as
  M1.1's exit mapping; scratch/creds reaped.

## Requirements *(mandatory)*

- **FR-001**: The declaration MUST accept role `tool` with lifecycle `service`
  (M1.2 subset), alongside M1.1's `agent`/`service`.
- **FR-002**: soulrealm MUST mint a tool workload a credential scoped to *serve*
  its capability — publish replies and subscribe its service + discovery
  subjects — and nothing else (constitution II).
- **FR-003**: soulrealm MUST mint an agent (caller) a credential scoped to
  *discover and call* — request the service and discovery subjects, receive on
  its inbox — and nothing else.
- **FR-004**: A launched tool MUST be discoverable by name over the realm
  transport and answer request-reply calls.
- **FR-005**: soulrealm MUST support launching a **persistent** workload:
  start it, keep it running, and **stop** it on command — publishing `work.open`
  + `work.claim` at launch and `work.done` (clean stop) or `work.abandon`
  (crash/failed-readiness) at stop (constitution V).
- **FR-006**: Stopping (or a crash of) a tool MUST reap its process, scratch
  dir, and credential (FR-010 of M1.1, extended to services).
- **FR-007**: Neither the tool's nor the agent's credential MUST permit acting
  outside its scope — enforced by the server (as SC-003 in M1.1).

### Key Entities

- **Tool workload** — role `tool`, lifecycle `service`; a persistent process
  serving a capability over request-reply.
- **Service scope** / **Caller scope** — two credential shapes: one to serve a
  capability, one to discover-and-call it.
- **Running workload handle** — the launch/stop control surface for a persistent
  workload (the runner's new capability).

## Success Criteria *(mandatory)*

- **SC-001**: An agent discovers a launched tool by name and receives its reply
  (uppercase `"hi"` → `"HI"`), with no manual wiring of subjects by the operator.
- **SC-002**: The tool's `work.open`/`work.claim` appear at launch and
  `work.done` at stop, on the topic; no soulrealm-private control subject.
- **SC-003**: The tool credential cannot publish outside its service scope and
  the agent credential cannot outside its caller scope — both denied by an
  operator-mode server.
- **SC-004**: Stopping the tool reaps process, scratch, and credential; a
  crash yields `work.abandon` and the same reaping.

## Assumptions

- Reuses everything from M1.1 (declaration, minter, native backend, the runner's
  op mapping); this feature *extends* them, not replaces.
- Discovery uses NATS's built-in service mechanism (the `micro` framework's
  `$SRV` discovery) over the realm account — no registry service is built.
- The tool and agent artifacts are minimal reference programs (an uppercase
  tool; a caller agent), the M1.2 analogues of `agent-echo`.
- Rich discovery (capability search, versioning, health policy) and the
  `function`/`job` lifecycles remain later work.
- Single node; soulstream-only scope (episode 0003) still holds.

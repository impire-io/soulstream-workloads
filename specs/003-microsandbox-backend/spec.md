# Feature Specification: Second backend — the same declarations under microVM isolation

**Feature Branch**: `003-microsandbox-backend`
**Created**: 2026-07-28
**Status**: Draft
**Input**: Roadmap Phase 1 M1.3, from design
[`hq/02-DESIGN/0001-soulrealm-runtime.md`](../../hq/02-DESIGN/0001-soulrealm-runtime.md)
§6/§9. Backend chosen by the operator: **microsandbox** (microVM isolation).

> **Open amendment (recorded here, propagated on landing):** the roadmap and
> design §9 name "Docker or Firecracker" as the second backend. The operator
> chose **microsandbox** instead: it provides microVM-grade isolation (a guest
> kernel, not a shared one) *and* runs on both the macOS development machine
> and Linux nodes — Firecracker cannot run on macOS, and Docker on macOS is a
> VM hiding behind a daemon with weaker per-workload isolation. Constitution
> III is about the *seam*, not the brand; the roadmap item and design §9 are
> amended openly when this feature lands.

Soulrealm runs the **same agent and tool workloads from M1.1/M1.2 under a
second isolation backend** — each workload inside its own microVM sandbox —
while the declarations stay byte-identical and the lifecycle stays legible as
the same work ops on the same topic. This proves constitution III: how a
workload is isolated is a node-side choice, never a property of the workload.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The agent runs unchanged, one wall thicker (Priority: P1)

An operator takes the exact agent declaration that ran natively in M1.1 and
runs it on a node whose runner is configured to use the sandbox backend. The
agent launches inside its own microVM, reaches the realm transport with only
its scoped credential, posts a turn attributed to its persona, and its
lifecycle appears as `work.open`/`work.claim`/`work.done` on the topic —
indistinguishable, from the realm's point of view, from the native run.

**Why this priority**: This is the milestone's entire point — the smallest
demonstration that the declaration says nothing about isolation and the
backend seam holds. Nothing less proves constitution III.

**Independent Test**: Run the M1.1 agent declaration under the sandbox
backend; assert the persona-attributed turn and the three work ops appear on
the topic, and that the declaration used is byte-identical to the native one.

**Acceptance Scenarios**:

1. **Given** the unmodified M1.1 agent declaration and a runner configured for
   the sandbox backend, **When** soulrealm launches it, **Then** the agent
   runs inside a sandbox, posts its turn as its persona, and
   `work.open`/`work.claim`/`work.done` appear on the topic.
2. **Given** the declarations used in the native and sandbox runs, **When**
   they are compared, **Then** the diff is empty.
3. **Given** the agent running inside the sandbox, **When** it probes the
   host outside its provided working area (a path that the native backend
   could read), **Then** the probe fails — the isolation boundary is real,
   not cosmetic.

---

### User Story 2 - The tool serves from inside the sandbox (Priority: P2)

The operator runs the M1.2 pair — a persistent tool service and the agent
that discovers and calls it — under the sandbox backend. The tool serves its
capability from inside its own sandbox; the agent discovers it by name and
gets its reply, exactly as in M1.2. Stopping the tool still yields
`work.done` and reaps everything.

**Why this priority**: The service lifecycle (launch, keep running, stop) is
the harder half of the runner's contract; proving it survives the backend
swap shows the seam covers both lifecycles, not just run-to-completion.

**Independent Test**: Launch the M1.2 uppercase tool under the sandbox
backend, run the caller agent, assert `"hi"` → `"HI"`; stop the tool, assert
`work.done` and full reaping.

**Acceptance Scenarios**:

1. **Given** the unmodified M1.2 tool declaration launched under the sandbox
   backend, **When** the caller agent discovers it by name and sends `"hi"`,
   **Then** the agent receives `"HI"`.
2. **Given** the running sandboxed tool, **When** soulrealm stops it,
   **Then** `work.done` appears on the topic and the sandbox, scratch dir,
   and credential are all gone.

---

### User Story 3 - Failure and cleanup parity (Priority: P3)

A workload that crashes inside its sandbox, never becomes ready, or ignores a
stop request is handled exactly as under the native backend: the run ends in
`work.abandon` with the same exit legibility, stop escalates until the
sandbox is torn down, and no sandbox instance, scratch dir, or credential
outlives its workload.

**Why this priority**: Constitution V (legible lifecycle) and II (credentials
die with the workload) must not degrade behind a thicker wall; a backend that
leaks sandboxes or hides failures would be worse than no second backend.

**Independent Test**: Run a workload that exits nonzero inside the sandbox;
assert `work.abandon` and that no sandbox instance remains afterwards.

**Acceptance Scenarios**:

1. **Given** a workload that exits nonzero inside its sandbox, **When** it
   dies, **Then** `work.abandon` appears with the failure legible, and the
   sandbox, scratch, and credential are reaped.
2. **Given** a sandboxed workload that ignores termination, **When** soulrealm
   stops it and the grace period expires, **Then** the sandbox is forcibly
   torn down and the lifecycle still closes on the topic.

### Edge Cases

- **Node cannot run sandboxes** (virtualization unavailable, sandbox runtime
  not installed) — launch fails immediately and legibly; no half-launched
  workload, no op sequence left open.
- **Workload inside the sandbox cannot reach the realm transport** — the
  readiness bound expires, the run ends in `work.abandon`, and the sandbox is
  reaped (same as M1.2's never-ready case).
- **Sandbox startup is slower than a native process** — launch-to-ready must
  still fit the existing readiness bound; slower isolation must not silently
  loosen the lifecycle.
- **Artifact cannot run inside the sandbox's environment** (built for the
  wrong OS/architecture) — the failure surfaces as `work.abandon` with the
  exit legible, not as a hang.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The isolation backend MUST be selected node-side (runner
  configuration); the declaration MUST NOT contain any backend-selecting or
  backend-specific field (constitution III).
- **FR-002**: The second backend MUST run each workload inside its own
  sandbox with a real isolation boundary: the workload cannot see host
  processes or host filesystem paths beyond what soulrealm provides to it.
- **FR-003**: The second backend MUST fulfil the existing backend contract —
  launch a workload with its injected scoped credential, report how it ended,
  and stop it on command with escalation — with no change to that contract's
  shape visible to the runner.
- **FR-004**: A sandboxed workload MUST reach the realm transport using only
  its scoped persona credential; the sandbox MUST NOT grant it any broader
  network identity or credential (constitution II).
- **FR-005**: Lifecycle publication MUST be identical across backends: the
  same `work.open`/`work.claim`/`work.done`/`work.abandon` mapping on the
  topic, emitted by the runner, with no backend-private control channel
  (constitutions I and V).
- **FR-006**: Ending a sandboxed workload — clean exit, stop, or crash —
  MUST reap the sandbox instance, the scratch dir, and the credential;
  nothing about the run outlives it on the node.
- **FR-007**: Both reference workloads — the M1.1 agent and the M1.2
  tool-plus-caller — MUST run under the second backend with their
  declarations byte-identical to the native runs.
- **FR-008**: Both lifecycles the runner knows today — run-to-completion
  (agent) and persistent service (tool) — MUST be supported by the second
  backend.

### Key Entities

- **Sandbox backend** — the second implementation behind the backend seam;
  runs one workload per microVM sandbox.
- **Sandbox instance** — the per-workload isolated environment: created at
  launch, torn down at end-of-life, never shared between workloads.
- **Backend selection** — node-side runner configuration naming which backend
  launches workloads; the only place isolation is chosen.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The M1.1 agent declaration runs under the sandbox backend with
  a zero-byte diff, posting its persona-attributed turn with
  `work.open`/`work.claim`/`work.done` on the topic.
- **SC-002**: The M1.2 tool declaration runs under the sandbox backend with a
  zero-byte diff; the caller agent discovers it by name and `"hi"` → `"HI"`;
  stop yields `work.done`.
- **SC-003**: An isolation probe that succeeds under the native backend
  (reading a host path outside the workload's provided area) fails from
  inside the sandbox.
- **SC-004**: A workload crash inside the sandbox yields `work.abandon` with
  the exit legible; after any end-of-life (done or abandon), zero sandbox
  instances, scratch dirs, or credentials remain on the node.
- **SC-005**: The full M1.3 gate — both reference scenarios under the sandbox
  backend — passes on the development machine itself (no remote Linux host
  required), keeping the gate runnable by the operator (constitution VI).

## Assumptions

- **Microsandbox is the chosen sandbox runtime** (microVM isolation via
  libkrun; macOS and Linux). Its presence on a node is an operator-installed
  prerequisite, like the NATS server — soulrealm does not install it.
- **Artifact resolution is node-side provisioning, not declaration content.**
  The declaration references the artifact; resolving it to something the
  sandbox's guest environment can execute (e.g., a guest-OS build of the same
  reference program) is the node's job. Rebuilding an artifact for the guest
  does not count as a declaration change; the zero-diff criterion is about
  the declaration.
- **The native backend remains the default**; the sandbox backend is opt-in
  per runner. Backend selection stays out of the declaration by construction.
- **Reuses everything from M1.1/M1.2** — declaration, minter, runner op
  mapping, reference workloads; this feature adds a second implementation
  behind the existing seam and touches nothing above it.
- **Single node; soulstream-only scope** (episode 0003) still holds. Fleet
  scheduling, artifact registries, and further backends (Firecracker,
  Kubernetes) remain later horizons.
- **The roadmap/design amendment** (microsandbox instead of Docker or
  Firecracker) is recorded in this spec and propagated to
  `hq/03-IMPLEMENTATION/roadmap.md` and design 0001 §6/§9 when the feature
  lands, per the roadmap's discipline.

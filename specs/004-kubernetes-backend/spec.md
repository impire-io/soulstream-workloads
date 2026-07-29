# Feature Specification: Third backend — the same declarations as Kubernetes pods

**Feature Branch**: `004-kubernetes-backend`
**Created**: 2026-07-29
**Status**: Draft
**Input**: Roadmap Phase 2 M2.1, from design
[`hq/02-DESIGN/0002-kubernetes-backend.md`](../../hq/02-DESIGN/0002-kubernetes-backend.md)
(graduated from the `kubernetes-backend` research topic — all four
pre-registered bars measured PASS, episode
[0008](../../hq/04-JOURNEY/0008-kubernetes-backend.md)).

Soulrealm runs the **same agent and tool workloads from M1.1/M1.2 as pods on
a Kubernetes cluster** — one pod per workload, supervised by the runner —
while the declarations stay byte-identical and the lifecycle stays legible as
the same work ops on the same topic. The value is **reach**: Kubernetes is
the infrastructure organizations already operate, so this backend is where
soulrealm meets existing compute. The trade is recorded openly (episode
0008): a pod is *weaker* isolation than the microVM backend — this feature
buys adoption, not a thicker wall. The backend is a **backend, not a
scheduler**: it makes no placement decisions; cluster-side scheduling remains
a Fleet-era question.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The agent runs unchanged, on the cluster the org already has (Priority: P1)

An operator takes the exact agent declaration that ran natively in M1.1 (and
sandboxed in M1.3) and runs it on a node whose runner is configured to use
the Kubernetes backend against a cluster the operator already has. The agent
launches as a pod, reaches the realm transport with only its scoped
credential, posts a turn attributed to its persona, and its lifecycle appears
as `work.open`/`work.claim`/`work.done` on the topic — indistinguishable,
from the realm's point of view, from the native run.

**Why this priority**: This is the milestone's entire point — the third
independent proof that the declaration says nothing about isolation, on the
substrate organizations actually run. Nothing less delivers the adoption
value.

**Independent Test**: Run the M1.1 agent declaration under the Kubernetes
backend; assert the persona-attributed turn and the three work ops appear on
the topic, and that the declaration used is byte-identical to the native
control run.

**Acceptance Scenarios**:

1. **Given** the unmodified M1.1 agent declaration and a runner configured
   for the Kubernetes backend, **When** soulrealm launches it, **Then** the
   agent runs as a pod, posts its turn as its persona, and
   `work.open`/`work.claim`/`work.done` appear on the topic.
2. **Given** the declarations used in the native and Kubernetes runs,
   **When** they are compared, **Then** the diff is empty.

---

### User Story 2 - The tool serves from inside a pod (Priority: P2)

The operator runs the M1.2 pair — a persistent tool service and the agent
that discovers and calls it — under the Kubernetes backend. The tool serves
its capability from inside its own pod; the agent discovers it by name and
gets its reply, exactly as in M1.2. Stopping the tool still yields
`work.done` and reaps everything the launch created.

**Why this priority**: The persistent-service lifecycle (launch, keep
running, stop on command) is the harder half of the runner's contract;
proving it survives the third backend swap shows the seam covers both
lifecycles on a substrate whose natural instinct (restart, reschedule) must
be held off.

**Independent Test**: Launch the M1.2 uppercase tool under the Kubernetes
backend, run the caller agent, assert `"hi"` → `"HI"`; stop the tool, assert
`work.done` and full reaping.

**Acceptance Scenarios**:

1. **Given** the unmodified M1.2 tool declaration launched under the
   Kubernetes backend, **When** the caller agent discovers it by name and
   sends `"hi"`, **Then** the agent receives `"HI"`.
2. **Given** the running pod-hosted tool, **When** soulrealm stops it,
   **Then** `work.done` appears on the topic and the pod, the delivered
   credential, and any staged artifact are all gone.

---

### User Story 3 - Failure and cleanup parity (Priority: P3)

A workload that crashes inside its pod, never becomes ready, or ignores a
stop request is handled exactly as under the native and sandbox backends:
the run ends in `work.abandon` with the exit legible, stop escalates from a
polite request to forcible termination, and nothing created for the run —
pod, delivered credential, staged artifact — outlives the workload. The
cluster is never allowed to quietly restart or resurrect a workload the
runner believes is dead.

**Why this priority**: Constitution V (legible lifecycle) and II
(credentials die with the workload) must not degrade on a substrate that has
its own opinions about restarts; a backend that leaks pods or hides failures
would be worse than no third backend.

**Independent Test**: Run a workload that exits nonzero inside its pod;
assert `work.abandon` and that no pod, credential, or staged artifact
remains afterwards.

**Acceptance Scenarios**:

1. **Given** a workload that exits nonzero inside its pod, **When** it dies,
   **Then** `work.abandon` appears with the failure legible, and the pod,
   delivered credential, and staged artifact are reaped.
2. **Given** a pod-hosted workload that ignores termination, **When**
   soulrealm stops it and the grace period expires, **Then** the workload is
   forcibly terminated and the lifecycle still closes on the topic.
3. **Given** any end of life (done or abandon), **When** the run is over,
   **Then** zero workload pods and zero delivered credentials remain on the
   cluster.

---

### User Story 4 - The scoped credential holds from inside the pod (Priority: P4)

A workload running in a pod receives its persona-scoped credential
confidentially, connects to the realm's transport over the pod's ordinary
network path, and is held to its scope by the realm: what its persona may
touch succeeds, anything beyond is denied by the server — exactly as if it
ran natively. The credential is readable only by that workload's pod and is
removed when the workload ends.

**Why this priority**: One identity, no privileged tier (constitution II)
must survive the delivery mechanism changing from a local file to
cluster-carried delivery; this was research Bar 4 and it is the security
floor of the feature.

**Independent Test**: Run a scope-probe workload in a pod against an
enforcing (operator-mode) realm; assert its in-scope action succeeds and its
out-of-scope action is denied by the server.

**Acceptance Scenarios**:

1. **Given** a pod-hosted workload with its minted credential, **When** it
   acts within its persona's scope, **Then** the action succeeds.
2. **Given** the same workload, **When** it attempts an action outside its
   scope, **Then** the realm's server denies it.
3. **Given** the workload's end of life, **When** the pod is reaped, **Then**
   the delivered credential is gone from the cluster.

### Edge Cases

- **Cluster unreachable or misconfigured at launch** — the launch fails
  immediately and legibly (start failure → `work.abandon`, no dangling
  claim); no half-launched workload.
- **Artifact built for the wrong platform** — refused node-side *before*
  launch with a legible error. (Measured in research: letting it into the
  pod produces an unreadable in-pod failure.)
- **Realm transport requires TLS and the pod image cannot verify it** — the
  workload never becomes ready; the run ends in `work.abandon` and is
  reaped. The shipped default image MUST be able to verify TLS (research-
  measured requirement).
- **The cluster deletes or evicts the pod out-of-band** — the runner
  observes the death and closes the lifecycle (`work.abandon`); supervision
  survives external interference, and the cluster's restart instincts never
  produce a second copy of the workload.
- **Slow image pull / cold start** — launch-to-ready must still fit the
  existing readiness bound; slower substrate must not silently loosen the
  lifecycle.
- **A workload legitimately exits with a code above 128** — reported with
  the named limitation: indistinguishable from a signal death on this
  substrate (design 0002 §5); the lifecycle op is still correct (nonzero →
  abandon).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The isolation backend MUST be selected node-side (runner
  configuration); the declaration MUST NOT contain any backend-selecting,
  image-naming, or Kubernetes-specific field (constitution III).
- **FR-002**: The backend MUST run each workload as exactly one pod, never
  shared between workloads, created at launch and removed at end of life.
- **FR-003**: The backend MUST fulfil the existing backend contract — launch
  a workload with its injected scoped credential, report how it ended, stop
  it on command with escalation — with no change to the contract's shape:
  the runner, minter, and declaration components remain untouched.
- **FR-004**: A pod-hosted workload MUST reach the realm transport using
  only its scoped persona credential; the credential MUST be delivered
  confidentially to that workload's pod alone and MUST be removed when the
  workload ends (constitution II).
- **FR-005**: Lifecycle publication MUST be identical across backends: the
  same `work.open`/`work.claim`/`work.done`/`work.abandon` mapping on the
  topic, emitted by the runner; the backend publishes no ops and owns no
  control channel (constitutions I and V).
- **FR-006**: Ending a pod-hosted workload — clean exit, stop, or crash —
  MUST reap everything the launch created: the pod, the delivered
  credential, and any staged artifact; nothing about the run outlives it.
- **FR-007**: Both reference workloads — the M1.1 agent and the M1.2
  tool-plus-caller — MUST run under the Kubernetes backend with their
  declarations byte-identical to the native runs.
- **FR-008**: Both lifecycles the runner knows — run-to-completion (agent)
  and persistent service (tool) — MUST be supported, with the cluster's own
  restart behaviour disabled so supervision stays with the runner.
- **FR-009**: The artifact MUST reach the pod under the established
  node-side provisioning convention (stable declared reference, per-run
  content) with no declaration change; the node MUST verify the artifact is
  executable for the pod's platform *before* launch.
- **FR-010**: The backend MUST make no placement or scheduling decisions
  beyond creating the pod on its one configured cluster and namespace
  (backend, not scheduler).
- **FR-011**: How a workload ended MUST stay legible: exit codes reported
  faithfully; signal deaths reported as such, with the substrate's
  code-above-128 ambiguity recorded as a named limitation, not hidden.
- **FR-012**: The default quality gate MUST stay hermetic (no cluster, no
  external dependency); the real-cluster proof MUST be an opt-in gate target
  runnable on the development machine (constitution VI, the M1.3 pattern).

### Key Entities

- **Kubernetes backend** — the third implementation behind the backend seam;
  runs one workload per pod on one operator-provided cluster.
- **Workload pod** — the per-workload isolated environment: created at
  launch, supervised by the runner, removed at end of life, never shared,
  never restarted by the cluster.
- **Delivered credential** — the minted persona-scoped credential as it
  travels to the pod: confidential to the workload, lifetime bounded by the
  workload's, removed at reap time.
- **Generic runner image** — the node-configured container image every
  workload pod uses; able to fetch-and-execute a provisioned artifact and to
  verify TLS toward the realm transport; never named in a declaration.
- **Staged artifact** — the per-run copy of the workload's artifact placed
  where the pod can fetch it; provisioned node-side, reaped with the run.
- **Backend selection** — node-side runner configuration naming which
  backend launches workloads; the only place isolation is chosen.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The M1.1 agent declaration runs under the Kubernetes backend
  with a zero-byte diff against the native control run, posting its
  persona-attributed turn with `work.open`/`work.claim`/`work.done` on the
  topic.
- **SC-002**: The M1.2 tool declaration runs under the Kubernetes backend
  with a zero-byte diff; the caller agent discovers it by name and `"hi"` →
  `"HI"`; stop yields `work.done`.
- **SC-003**: A workload crash inside its pod yields `work.abandon` with the
  exit legible; after every end of life (done or abandon), zero workload
  pods, delivered credentials, or staged artifacts remain.
- **SC-004**: A scope-probe workload run inside a pod against an enforcing
  realm succeeds in-scope and is denied out-of-scope by the server, with its
  credential delivered confidentially and gone after reap.
- **SC-005**: The default gate (`make fmt && make test && make lint`) passes
  with no cluster present; the opt-in real-cluster target passes on the
  development machine against a local cluster (constitution VI).

## Assumptions

- **The cluster is an operator-provided prerequisite**, like the NATS server
  and the sandbox runtime before it: soulrealm connects to one configured
  cluster and namespace per runner and does not install, provision, or
  manage clusters. Single-architecture clusters are assumed; a
  heterogeneous-architecture cluster is out of scope (the node refuses a
  platform-mismatched artifact rather than resolving per-node).
- **Artifact resolution is node-side provisioning, not declaration content**
  (the M1.3 precedent): the declaration references the artifact; providing
  content the pod's platform can execute is the node's job. The *channel*
  by which staged artifact bytes reach the pod (served node-side vs the
  realm's own object store) is a design-0002 `[O]` decided in the plan
  phase; the fetch-then-execute shape inside a generic image is fixed by the
  research.
- **The cluster-client internal** (a client library vs supervising the
  cluster's own CLI, the M1.3 pattern) is a design-0002 `[O]` decided in the
  plan phase; the seam is indifferent.
- **Credential-at-rest exposure inside the cluster is a named
  consideration**, bounded by the credential's short mint lifetime;
  encrypting the cluster's secret store at rest is the operator's cluster
  concern, not soulrealm's.
- **The shipped default image can verify TLS** toward the realm transport
  (research-measured: an image without a CA trust store cannot reach a TLS
  realm); the image remains node-side configuration.
- **The native backend remains the default**; the Kubernetes backend is
  opt-in per runner, selected the same node-side way as the sandbox backend
  (M1.3 convention).
- **Isolation is weaker than the microVM backend and that is accepted**:
  the feature's value is adoption on existing infrastructure (episode 0008's
  adversarial note); no isolation-strength claim is made or tested beyond
  the credential scope holding.
- **Reuses everything from M1.1–M1.3** — declaration, minter, runner op
  mapping, reference workloads, backend seam; this feature adds a third
  implementation behind the existing seam and touches nothing above it.
- **Single node; soulstream-only scope** (episode 0003) still holds. Fleet
  scheduling, multi-cluster, artifact registries, and further backends
  (Docker, Firecracker) remain later horizons.

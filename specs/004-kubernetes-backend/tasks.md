# Tasks: Third backend — the same declarations as Kubernetes pods

**Input**: Design documents from `/specs/004-kubernetes-backend/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Included — the spec's success criteria are test-shaped (SC-001…005)
and constitution VI makes the gate part of "done". Hermetic unit tests
(fake clientset + in-process OCI registry) accompany every implementation
task; the real-cluster proof is its own gate target per research D7.

**Organization**: By user story from spec.md — US1 (agent unchanged on the
org's cluster), US2 (tool from inside a pod), US3 (failure & cleanup
parity), US4 (scoped credential holds from inside the pod).

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: Dependencies and the operator-side e2e environment.

- [x] T001 Add dependencies to `go.mod`: `k8s.io/client-go`, `k8s.io/api`,
      `k8s.io/apimachinery` (v0.36.x) and
      `github.com/google/go-containerregistry`; `go mod tidy`; confirm
      `make check` still green before any new code (research D1/D2)
- [x] T002 [P] Create `scripts/kind-registry.sh` (`up`/`down`): kind cluster
      `soulrealm-k8s` + local `registry:2` container wired per the
      documented kind-with-registry pattern (containerd registry config,
      shared network, `localhost:5001` on the host); the script MUST ensure
      the push reference and the in-cluster pull reference are the same
      name (the name-parity gotcha the documented pattern exists to solve);
      referenced by quickstart.md and expected by `make test-k8s` — the
      operator prerequisite in script form (research D7, analysis C1)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The shared rewrite helper, the backend package skeleton, and
both hermetic test seams — every story builds on these.

- [x] T003 Extract `backend/natsurl/natsurl.go` (+ `natsurl_test.go`) from
      `backend/msb`'s unexported `rewriteServers`/`rewriteServer`/
      `isLoopback`; `backend/msb` adopts it with behavior pinned by msb's
      existing tests staying green (plan structure decision — one rewrite
      implementation, two consumers)
- [x] T004 Create `backend/k8s/k8s.go` skeleton: `Backend{Client
      kubernetes.Interface, Namespace, Registry, BaseImage, HostAlias}`
      with defaults (`default`, required, `alpine:3.22`, unset), `New()`,
      RFC 1123 name sanitization (`soulrealm-<workitem-id>` +
      `app.kubernetes.io/managed-by` label), node-side ELF check, and the
      pod/Secret spec builders per contracts/k8s-mapping.md (single
      container, digest-pinned image ref, `command ["/workload", args…]`,
      `restartPolicy: Never`, grace = 5 s, `emptyDir` `/scratch` workdir,
      Secret ro at `/creds`, env = workload-env contract via
      `backend/natsurl`) — no supervision yet
- [x] T005 [P] Create `backend/k8s/image.go` (+ `image_test.go`): per-run
      OCI assembly — artifact bytes as a layer on `BaseImage` at
      `/workload`, entrypoint `/workload` — and digest-pinned push tagged
      with the work-item id (go-containerregistry); unit tests against an
      in-process registry (`go-containerregistry/pkg/registry`) asserting
      layer content, entrypoint, tag, and returned digest (research D2)
- [x] T006 [P] Create the fake-clientset harness in
      `backend/k8s/k8s_test.go`: helpers that drive the fake's watch
      (status-update sequences, Deleted events) so supervision is testable
      hermetically (research D1/D7)

**Checkpoint**: `go build ./...` green; specs and images constructible and
asserted with no cluster and no daemon.

---

## Phase 3: User Story 1 — The agent runs unchanged, on the cluster the org already has (P1) 🎯 MVP

**Goal**: The byte-identical M1.1 agent declaration runs as a pod: turn
posted as its persona, `work.open/claim/done` on the topic, native control
arm proving the zero-byte diff (SC-001).

**Independent Test**: `make test-k8s` runs `TestK8sLaunchAgentEndToEnd`
green; unit tests prove spec shape, mapping, and reap hermetically.

- [x] T007 [US1] Complete supervision + `handle` in `backend/k8s/k8s.go`:
      watch goroutine started by `Start` (field selector on the pod name,
      capture `ContainerStateTerminated` on every update — research D4),
      `Wait` (terminal phase or Deleted → exit mapping per data-model.md
      table incl. 128+n signal inference and no-state → `Code: -1` → reap:
      delete pod grace-0 idempotent + delete Secret → status;
      `sync.Once`), `Stop` (delete with `gracePeriodSeconds` =
      min(5 s, ctx remaining)), start-failure rollback (unwind Secret/pod,
      error, nothing left on the cluster)
- [x] T008 [P] [US1] Hermetic unit tests in `backend/k8s/k8s_test.go`
      against the fake: pod spec shape (single digest-pinned container,
      command+args, restartPolicy, grace, mounts, workdir, label), env
      block exactly the workload-env contract with rewritten servers and
      nothing inherited, Secret holds the creds-file bytes, name
      sanitization cases, ELF refusal pre-cluster-call, exit mapping table
      (0 / 3 / 137→`killed` / Deleted-no-state→−1), Stop grace derivation
      from ctx, reap idempotency, start-failure rollback
- [x] T009 [US1] E2E `TestK8sLaunchAgentEndToEnd` in
      `integration/k8s_e2e_test.go` (build tag `k8s_e2e`): the
      launch_test.go scenario with `Backend: k8s` and
      `buildCmdLinux(agent-echo)`; ONE declaration value marshalled and
      byte-compared across a native control run and the pod run (the msb
      zero-diff pattern); suite NATS bound `0.0.0.0` with the host alias so
      the loopback rewrite is exercised for real; artifact pushed to the
      local registry and pulled by kind (SC-001, US1 acceptance 1+2)
- [x] T010 [US1] Add `test-k8s` target to `Makefile`
      (`go test -tags k8s_e2e -count=1 ./integration/ -run 'TestK8s'`,
      expects `scripts/kind-registry.sh up` done) and the build-tag header
      in k8s_e2e_test.go

**Checkpoint**: MVP — constitution III proven a third time for the
run-to-completion lifecycle, on the substrate organizations already run.

---

## Phase 4: User Story 2 — The tool serves from inside a pod (P2)

**Goal**: The M1.2 tool pair works with the tool inside a pod: discovery by
name, `"hi"`→`"HI"`, stop → `work.done` + cluster-side reaping (SC-002).

**Independent Test**: `TestK8sAgentCallsToolEndToEnd` green under
`make test-k8s`.

- [x] T011 [US2] E2E `TestK8sAgentCallsToolEndToEnd` in
      `integration/k8s_e2e_test.go`: tool_test.go scenario with the k8s
      backend and `buildCmdLinux(tool-upper)`; discovery retry window
      ≥ 60 s (research: cold image pull margin); after `Stop`: assert
      `work.done` and the managed-by label sweep shows zero pods and zero
      Secrets (SC-002, US2 acceptance 1+2)

**Checkpoint**: Both lifecycles (run-to-completion + persistent service)
proven under the third backend.

---

## Phase 5: User Story 3 — Failure and cleanup parity (P3)

**Goal**: Crash → `work.abandon` with the exit legible; the cluster's
restart/interference instincts never resurrect a workload; nothing on the
cluster outlives any end of life (SC-003).

**Independent Test**: `TestK8sCrashAbandons` and
`TestK8sOutOfBandDeletion` green under `make test-k8s`; Deleted-event
handling covered hermetically.

- [x] T012 [US3] E2E `TestK8sCrashAbandons` in
      `integration/k8s_e2e_test.go`: workload exiting nonzero inside its
      pod → runner records `work.abandon`; afterwards the label sweep shows
      zero pods/Secrets (SC-003, US3 acceptance 1+3)
- [x] T013 [P] [US3] Hermetic unit test in `backend/k8s/k8s_test.go`: the
      fake watch delivers a Deleted event with no termination state ever
      observed → `Wait` returns the uncoded failure (`Code: -1`) and reaps
      — the out-of-band-interference path (US3 edge case, research D4)
- [x] T014 [P] [US3] E2E `TestK8sOutOfBandDeletion` in
      `integration/k8s_e2e_test.go`: delete the running pod via the
      cluster (not the runner) mid-run → the run still closes as
      `work.abandon` and no second copy of the workload ever appears
      (US3 acceptance — supervision survives external interference)

**Checkpoint**: Failure legibility and cleanup parity proven end to end.

---

## Phase 6: User Story 4 — The scoped credential holds from inside the pod (P4)

**Goal**: The minted credential reaches the pod as a Secret, the workload
connects over the pod's real network path, and an enforcing realm allows
in-scope and denies out-of-scope actions — from inside the pod (SC-004).

**Independent Test**: `TestK8sScopeEnforcedFromPod` green under
`make test-k8s`.

- [x] T015 [US4] Extend `internal/natstest` with bind-address options on
      **both** `StartJetStream` and `StartOperator` (defaults stay
      `127.0.0.1`; existing callers unchanged) so e2e servers are reachable
      from pods via the host alias — T009 needs the JetStream half, so do
      this before US1's e2e despite its US4 home (analysis I1)
- [x] T016 [US4] E2E `TestK8sScopeEnforcedFromPod` in
      `integration/k8s_e2e_test.go`: operator-mode in-process NATS bound
      `0.0.0.0`; inline-built probe workload (the research Bar 4 probe
      shape: in-scope publish on its allowed subject succeeds, out-of-scope
      publish draws the server's permissions violation, exit code encodes
      the verdict) run under the k8s backend with the credential delivered
      as the Secret; after reap assert the Secret is gone (SC-004, US4
      acceptance 1–3)

**Checkpoint**: All four stories independently green.

---

## Phase 7: Polish & Cross-Cutting

**Purpose**: Node-side selection, docs, the gate, and the landing
bookkeeping.

- [x] T017 Wire backend selection into `cmd/soulrealm/main.go`:
      `SOULREALM_BACKEND` = `native` (default) | `msb` | `k8s`; `k8s` reads
      `SOULREALM_K8S_NAMESPACE` / `SOULREALM_K8S_REGISTRY` (required) /
      `SOULREALM_K8S_BASE_IMAGE` / `SOULREALM_K8S_HOST_ALIAS` /
      `SOULREALM_K8S_CONTEXT` and resolves kubeconfig via client-go's
      standard loading rules; unknown backend or missing registry errors
      before any op is published (FR-001); config parsing factored and
      unit-tested
- [x] T018 [P] Verify `specs/004-kubernetes-backend/quickstart.md` against
      the implementation (commands, env var names, script name); fix any
      drift
- [x] T019 Full gate: `make check` hermetic (verify green with no cluster,
      no registry, no kubeconfig context present — no hidden dependency)
      and `make test-k8s` (real kind + registry) — both green, nothing
      skipped (SC-005)
- [x] T020 Land per roadmap discipline, same merge: roadmap.md Phase 2/M2.1
      closed with the measured outcome; design 0002 §3/§6 amended with the
      OCI-registry decision (the spec's open-amendment note propagated) and
      §5/§6 for anything else that drifted; design 0001 §6 backend list
      updated; CLAUDE.md status; spec.md Status → Shipped; journey episode
      via /journey-log; signed commits

---

## Dependencies & Execution Order

- **Setup (T001–T002)** → **Foundational (T003–T006)** → stories. T002 is
  only needed before the first e2e run (T009), not before unit work.
- **US1 (T007–T010)**: the MVP. T007 needs T004 (+T005 for the image ref,
  T003 for the rewrite); T008 parallel with T009 once T007 lands; T009
  needs T002 + T005 + T015's `StartJetStream` bind option; T010 with T009.
- **US2 (T011)**: needs T007; independent of US1's e2e tests.
- **US3 (T012–T014)**: T012/T014 need T007; T013 needs T006+T007; all
  independent of US1/US2 e2e.
- **US4 (T015–T016)**: T015 is independent (natstest only); T016 needs
  T007 + T015.
- **Polish (T017–T020)**: T017 anytime after T004; T018 after stories;
  T019 after everything; T020 last.

## Parallel Opportunities

- T002 ∥ T001; T005 ∥ T006 (different files); T004 → then T005/T006 in
  parallel.
- After T007: T008 ∥ T009; T013 ∥ T012 ∥ T014; T015 ∥ any US1–US3 work.
- T018 ∥ T017.
- Single-developer reality: execute in ID order; the [P] markers mainly
  mark safe re-ordering points.

## Implementation Strategy

MVP = Phase 1 + 2 + US1 (T001–T010): proves constitution III end to end for
the agent on a real cluster through the real registry path. US2–US4 are
additive e2e coverage on the same backend code (US4 adds one natstest
option); Polish closes FR-001 and the milestone bookkeeping. Stop and
validate at every checkpoint (run `make fmt && make test && make lint`);
the M2.1 exit gate is T019's two commands green.

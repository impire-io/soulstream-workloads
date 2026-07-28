# Tasks: Second backend — the same declarations under microVM isolation

**Input**: Design documents from `/specs/003-microsandbox-backend/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Included — the spec's success criteria are test-shaped (SC-001…005)
and constitution VI makes the gate part of "done". Hermetic unit tests
accompany every implementation task; the real-microVM proof is its own story
phase per research D6.

**Organization**: By user story from spec.md — US1 (agent unchanged), US2
(tool from inside the sandbox), US3 (failure & cleanup parity).

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup

**Purpose**: Everything both backends' proof needs on this node.

- [ ] T001 Verify msb 0.6.7 present and healthy on the node (`msb doctor`
      green; alpine image pulled so e2e boots are warm) — no repo change,
      recorded in research.md's measured-environment section
- [ ] T002 Add `buildCmdLinux` helper (GOOS=linux, host GOARCH,
      CGO_ENABLED=0 build of a cmd/ artifact) to
      `integration/helpers_test.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The backend package skeleton and its test seam — every story
builds on it.

- [ ] T003 Create `backend/msb/msb.go`: `Backend{Image, MsbPath, HostAlias}`
      with defaults (`alpine`, `msb`, `host.microsandbox.internal`), `New()`,
      and the `Start` skeleton: scratch-dir + creds-file creation identical
      to native (shared shape, not shared code — native stays untouched),
      sandbox name `soulrealm-<base(ScratchDir)>`, `msb run` argv
      construction per contracts/backend-seam.md (`--no-tty`, `--name`,
      `-v scratch:/scratch`, `-v artifactDir:/artifact:ro`, `-w /scratch`,
      `--net host`, `-e` env block, image, `-- /artifact/<bin> args…`)
- [ ] T004 [P] Implement loopback→HostAlias NATS URL rewrite in
      `backend/msb/msb.go` (`rewriteServers`: 127.0.0.1/::1/localhost hosts
      only, others pass through — data-model.md validation rules)
- [ ] T005 [P] Create stub-msb test harness in `backend/msb/msb_test.go`: a
      helper that writes a fake `msb` shell script into t.TempDir() which
      records argv to a file and exits with a scripted code/delay — the
      hermetic seam from research D6

**Checkpoint**: `go build ./...` green; unit tests can drive Start end to end
with no real msb.

---

## Phase 3: User Story 1 — The agent runs unchanged, one wall thicker (P1) 🎯 MVP

**Goal**: The byte-identical M1.1 agent declaration runs inside a microVM:
turn posted as its persona, `work.open/claim/done` on the topic (SC-001), and
the isolation boundary is real (SC-003).

**Independent Test**: `make test-msb` runs `TestMsbLaunchAgentEndToEnd` +
`TestMsbIsolationBoundary` green; unit tests prove argv/env/rewrite hermetically.

- [ ] T006 [US1] Complete `handle` in `backend/msb/msb.go`: `Wait` (process
      wait → guest exit status via existing statusOf-style mapping, then
      `msb rm --force <name>` + scratch removal, sync.Once-idempotent),
      `Stop` (SIGTERM, 5 s grace, SIGKILL) — behavioral contract items 3/4/6
- [ ] T007 [P] [US1] Unit tests in `backend/msb/msb_test.go` against the stub:
      argv shape (mounts ro/rw, workdir, net profile, image, artifact path,
      name), env block exactly the workload-env contract with rewritten
      servers + in-guest creds path and nothing inherited from the parent
      env, creds file written 0600 into scratch, exit-code passthrough,
      Stop→SIGTERM escalation, start-failure leaves no scratch, Wait reaps
      scratch + invokes `msb rm`
- [ ] T008 [US1] E2E `TestMsbLaunchAgentEndToEnd` in
      `integration/msb_e2e_test.go` (build tag `msb_e2e`): the launch_test.go
      scenario verbatim but `Backend: msb.New()` and
      `buildCmdLinux(agent-echo)` — asserts persona turn + work.done (SC-001,
      acceptance 1); NATS listens on 127.0.0.1 so the rewrite is exercised
      for real
- [ ] T009 [P] [US1] E2E `TestMsbIsolationBoundary` in
      `integration/msb_e2e_test.go`: probe workload (inline-built) reads a
      host-side file outside its scratch — native backend run succeeds the
      read, msb run fails it (SC-003, acceptance 3)
- [ ] T010 [US1] Add `test-msb` target to `Makefile`
      (`go test -tags msb_e2e -count=1 ./integration/ -run 'TestMsb'`) and
      wire the build tag header into msb_e2e_test.go

**Checkpoint**: MVP — constitution III proven for the run-to-completion
lifecycle on this machine.

---

## Phase 4: User Story 2 — The tool serves from inside the sandbox (P2)

**Goal**: The M1.2 tool pair works with the tool inside a microVM: discovery
by name, `"hi"`→`"HI"`, stop → `work.done` + full reaping (SC-002).

**Independent Test**: `TestMsbAgentCallsToolEndToEnd` green under `make test-msb`.

- [ ] T011 [US2] E2E `TestMsbAgentCallsToolEndToEnd` in
      `integration/msb_e2e_test.go`: tool_test.go scenario with the msb
      backend and `buildCmdLinux(tool-upper)`; discovery retry window ≥ 15 s
      (research: cold-boot margin); after `Stop`: assert `work.done`, scratch
      gone, and `msb ls` has no `soulrealm-*` row (SC-002 + US2 acceptance 2)

**Checkpoint**: Both lifecycles (run-to-completion + persistent service)
proven under the second backend.

---

## Phase 5: User Story 3 — Failure and cleanup parity (P3)

**Goal**: Crash → `work.abandon` with the failure legible; stop escalation
works; nothing outlives any end-of-life (SC-004).

**Independent Test**: `TestMsbCrashAbandons` green under `make test-msb`;
escalation covered hermetically by the stub.

- [ ] T012 [US3] E2E `TestMsbCrashAbandons` in
      `integration/msb_e2e_test.go`: workload exiting nonzero inside the
      sandbox → runner records `work.abandon`; afterwards zero `soulrealm-*`
      sandboxes, scratch removed (SC-004, US3 acceptance 1)
- [ ] T013 [P] [US3] Stub-based unit test for stop-escalation in
      `backend/msb/msb_test.go`: fake msb that ignores SIGTERM → Stop
      escalates to SIGKILL within grace; Wait still reaps (US3 acceptance 2)

**Checkpoint**: All three stories independently green.

---

## Phase 6: Polish & Cross-Cutting

**Purpose**: Node-side selection, the zero-diff proof, docs, and the gate.

- [ ] T014 Wire backend selection into `cmd/soulrealm/main.go`:
      `SOULREALM_BACKEND` = `native` (default) | `msb` (+
      `SOULREALM_MSB_IMAGE` override); unknown value errors before any op
      (FR-001); factor a small `selectBackend` func with a unit test if main
      stays untestable
- [ ] T015 [P] Zero-diff assertion (SC-001/002 clause): in
      `integration/msb_e2e_test.go`, both e2e declarations constructed from
      one shared literal used verbatim for a native control-run in the same
      test file — the "diff is empty" proof is structural (same value), noted
      in a comment referencing spec acceptance US1-2
- [ ] T016 [P] Update `specs/003-microsandbox-backend/quickstart.md` if any
      command/flag drifted during implementation; confirm each quickstart
      command works as written
- [ ] T017 Full gate: `make check` (hermetic — verify it passes with
      `PATH` lacking msb to prove no hidden dependency) and `make test-msb`
      (real microVMs) — both green, nothing skipped (SC-005)
- [ ] T018 Land per roadmap discipline, same merge: roadmap.md M1.3 closed
      with measured outcome + the open amendment (microsandbox, not
      Docker/Firecracker); design 0001 §6 backend list + §9 acceptance-3
      wording amended; CLAUDE.md + README status lines; spec.md Status →
      Shipped; journey episode via /journey-log; signed commits

---

## Dependencies & Execution Order

- **Setup (T001–T002)** → **Foundational (T003–T005)** → stories.
- **US1 (T006–T010)**: the MVP; T006 blocks T008/T009; T007 parallel with
  T008 once T006 lands.
- **US2 (T011)**: needs T006 + T002; independent of US1's e2e tests.
- **US3 (T012–T013)**: needs T006; independent of US1/US2 e2e.
- **Polish (T014–T018)**: T014 anytime after T003; T015–T017 after all
  stories; T018 last.

## Parallel Opportunities

- T004 ∥ T005 (different concerns, same package but separable files/regions).
- T007 ∥ T008/T009 after T006; T009 ∥ T008 (different tests).
- T013 ∥ T012; T015/T016 ∥ each other.
- Single-developer reality: execute in ID order; the [P] markers mainly mark
  safe re-ordering points.

## Implementation Strategy

MVP = Phase 1 + 2 + US1 (T001–T010): proves constitution III end to end for
the agent. US2 and US3 are additive e2e coverage on the same backend code;
Polish closes FR-001 and the milestone bookkeeping. Stop-and-validate at
every checkpoint; the M1.3 exit gate is T017's two commands green.

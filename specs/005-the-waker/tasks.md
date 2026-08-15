# Tasks: The waker — notify-triggered invocation

**Input**: Design documents from `/specs/005-the-waker/`
**Prerequisites**: plan.md, research.md (D1–D8), data-model.md,
contracts/waker-contract.md

**Tests**: included — the spec's success criteria are measurable only
through them, and the research's measured traps (correlation, idempotency)
need standing regression guards.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Setup (cross-repo prerequisite + dependencies)

- [x] T001 **Core v0.8.3 (in `../soulstream-core`)**: add
  `Handle.PostTurnIdempotent(ctx, body string, mentions []string, opID
  string) (string, error)` to `topic/post.go`, riding `publishOpWith`'s
  existing preset-id arm (research D7); unit test in the
  `realm/dedup_test.go` pattern proving a same-id repost dedupes and
  returns the same op; core gate green; signed commit on core main;
  signed annotated tag `v0.8.3`. **Pushing core + the tag is the
  operator's act and must precede workloads CI** — local development
  rides T002's workspace.
- [x] T002 Workloads dependency wiring: create **untracked** `go.work`
  (`.` + `../soulstream-core`) — never committed (episode 0037's
  co-development pattern); `go get
  github.com/impire-io/soulstream-identity@v0.2.0` and
  `github.com/google/uuid` (direct require for UUIDv5); build green in
  workspace mode. *Amended during execution `[measured]`: workspace
  builds still read the go.mod of the version go.mod names (graph
  pruning), so an unpushed v0.8.3 pin breaks even workspace builds —
  go.mod stays at the published v0.8.0 during the branch (the workspace
  supplies v0.8.3's code) and the landing commit (T020) flips the pin
  to v0.8.3, after which the operator's push order (core + tag before
  workloads) makes CI whole.*

## Phase 2: Foundational (blocking prerequisites)

- [x] T003 [P] `waker/registration.go` + `registration_test.go`: `Config`
  (waker identity block + `[]Registration`), `Registration`, `Template`,
  `TerminalMap`; strict JSON load (`DisallowUnknownFields`, the
  declaration precedent); validation per contracts/waker-contract.md —
  exactly one credential lane, terminal mapping required, duration
  parsing, defaults (`max_deliver` 2, `run_timeout` 150s). Table tests in
  the `declaration_test.go` style.
- [x] T004 [P] `waker/correlate.go` + `correlate_test.go` (PURE, the
  `lifecycle.go` analog): `WakeOpID(notifyOpID) string` (UUIDv5, fixed
  namespace); `PostedDuringRun(before, after []Turn, persona) (string,
  bool)` set difference; `OutcomeFound(view []Turn, wakeOpID) bool` for
  the redelivery pre-check; outcome classification (success text /
  error status / no terminal / empty text). Regression cases from the
  measured trap: several mentions one topic; earlier reply must not
  satisfy a later wake.
- [x] T005 [P] `waker/harness.go` + `harness_test.go`: placeholder fill,
  fresh run dir under scratch, generated MCP config file (0600),
  `SOULSTREAM_*`-scrubbed child env, stdin closed, `Setpgid` +
  process-group SIGKILL on deadline, JSONL parse with dot-path
  terminal-event extraction. Table tests over both measured grammars
  (flat `type:result`, nested `msg.type:task_complete`), timeout, died,
  no-terminal, error-subtype, empty-text.
- [x] T006 [P] `cmd/harness-mock/main.go` (reference-workload style):
  flags `--grammar claude|codex`, `--reply <text>`, `--mode
  ok|die|hang|self-post`; `self-post` posts a turn as the agent through
  core (env contract) mid-run then emits a self-referential terminal —
  the measured fault (c).

## Phase 3: User Story 1 — a mention wakes an agent that isn't running (P1) 🎯 MVP

- [x] T007 [US1] `waker/wake.go`: the per-delivery protocol over injected
  narrow interfaces (`Prober`, `TopicReader` for materialise,
  `AgentPoster`/`WakerPoster`, `Invoker`): redelivery pre-check
  (`NumDelivered > 1` → `OutcomeFound` → ack), probe (refused/unreachable
  classes per research D4), context materialisation (before snapshot +
  anchoring body), invoke, discharge — reply via the **agent's client**
  `PostTurnIdempotent(text, nil, wakeOpID)`, ack strictly after; nak
  classes with distinct delays; discharge on `context.WithoutCancel`
  (the `runner.Running.base` pattern).
- [x] T008 [US1] `waker/waker.go`: package doc (constitutional why);
  `Waker.Serve(ctx)` — per registration `CreateOrUpdateConsumer` on
  `realm.NotifyStreamName` filtered `topic.NotifySubject(persona)`,
  AckExplicit, `AckWait = run_timeout + margin`, no server MaxDeliver;
  fetch loop per consumer goroutine; `log/slog` events (research D8):
  wake, refused, outcome, retry.
- [x] T009 [P] [US1] `waker/waker_test.go` + `wake_test.go`:
  call-sequence fakes (`runner_test.go` style) proving
  `probe,invoke,post,ack` (happy), `probe,nak` (refused — no invoke, no
  post), non-mention type → `ack` only, mint-fails → nak unreachable
  class.
- [x] T010 [US1] `cmd/soulstream-workloads/main.go`: replace the rigid arg check
  with a switch — `workload start <file>` unchanged, `waker serve
  <config-file>` new; config load + connect (waker context via
  `realm.Connect`); pure `wakerFromConfig` function; extend
  `main_test.go` for dispatch and config mapping.
- [x] T011 [US1] `integration/waker_test.go` — hermetic SC-001:
  `StartJetStream` + `Provision` + `StartTopic`; post a mention **before**
  the waker starts (the address-outlives-process premise); run the waker
  with a claude-grammar `harness-mock`; assert exactly one reply turn
  authored by the agent via `Materialise`; assert the wake op-id equals
  `WakeOpID(mention op-id)` (idempotency visible in the record).

**Checkpoint**: US1 alone is the MVP — an agent wakes and replies.

## Phase 4: User Story 2 — the conversation always learns the outcome (P2)

- [x] T012 [US2] Integration faults in `integration/waker_test.go`:
  `--mode die` (budget 2 → exactly one failure turn authored by the
  *waker's* persona, body naming agent + asker + reason);
  `--mode hang` with short `run_timeout` (same single-failure
  invariant); `--mode self-post` (exactly one turn — the mock's own; the
  waker posts nothing). After each: consumer info shows zero
  unprocessed, zero pending.
- [x] T013 [P] [US2] Unit: redelivery pre-check — fake reader returns a
  view already containing the wake op-id, `NumDelivered=2` → sequence is
  `ack` alone (no probe, no invoke).

## Phase 5: User Story 3 — address outlives process; revocation bites (P3)

- [x] T014 [US3] Integration backlog: three mentions in **one** topic
  posted with no waker running; start waker; assert three distinct
  replies (the measured multi-mention regression) and zero remaining
  deliveries.
- [x] T015 [US3] Probe classes: hermetic operator-mode test
  (`natstest.StartOperator`) — probe with a wrong credential is the
  refused class (nak-long, no op, no invoke); unit test maps transport
  failure to unreachable (nak-short). (Full token-lane revocation → 2ms
  refusal was measured on the product stack in research; the waker's
  contract here is class behavior, and the opt-in T018 path exercises
  the real lane.)
- [x] T016 [US3] Ephemeral lane: `EphemeralMinter` narrow interface in
  `waker/` (`MintEphemeral(role, user, pub, ttl, tags)`), lane selection
  in `wake.go` (mint → local nkey → creds file in run dir → probe/post
  through it); fake-backed unit tests; real
  `soulstream-identity/client` constructed only in `cmd/` when a
  registration declares the lane.

## Phase 6: User Story 4 — a new harness is configuration (P4)

- [x] T017 [US4] Integration: same waker process, second registration
  with the codex-grammar template (`--grammar codex`); mention both
  agents; each replies through its own grammar — one binary, two
  grammars, template-only difference (SC-004's regression guard).

## Phase 7: Polish & landing

- [x] T018 [P] `Makefile` `test-wake` opt-in target (build tag
  `wake_e2e`): real `claude -p` against a local realm per
  quickstart §3 — requires operator harness + auth, stays out of the
  hermetic gate.
- [x] T019 [P] Docs duty (S3): README waker section (what it is, the
  registration file, the two lanes, the bounded backlog); verify
  quickstart.md by hand against the built binary.
- [ ] T020 Full gate green in workspace mode (`make fmt && make test &&
  make lint`, nothing skipped); then land per roadmap discipline — merge
  `005-the-waker` to main; same session in soul-hq: journey episode,
  roadmap M3.2 ✅, design 0004 propagation (template home = waker config
  file per D2; §6 gains the idempotent-outcome mechanics per D7; §7
  notes the spike→design authorship correction shipped as specified).
  Remind the operator: push order is core (+ tag v0.8.3) before
  workloads.

## Dependencies & Execution Order

- T001 → T002 → everything else (the token lane and idempotent post are
  core API).
- Phase 2 tasks are mutually parallel [P]; T007/T008 depend on
  T003–T005; T010 depends on T008; T011 depends on T006 + T010.
- Phases 4–6 each depend on Phase 3 complete but are mutually
  independent; T016 is independent of T014/T015.
- T020 last, and only with every box above it checked.

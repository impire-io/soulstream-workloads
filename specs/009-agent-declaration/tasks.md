# Tasks: Agent declaration — wake, instructions, capabilities

**Input**: Design documents from `/specs/009-agent-declaration/`
**Prerequisites**: plan.md, spec.md, contracts/wake-kinds.md

**Tests**: included — the constitution's all-green gate (S5) plus design
0005's acceptance criteria as measured integration evidence.

**Organization**: tasks grouped by user story; US1 (the four kinds) is the
MVP and US3 (safety) rides the same engine change.

## Phase 1: Setup

No project setup needed — the feature grows existing packages plus one new
package in the module.

## Phase 2: Foundational (blocking prerequisites)

- [x] T001 Grow `declaration/declaration.go`: `Instructions`,
  `Capabilities`, `WakeEntry` (per-kind fields + `EffectiveTypes`),
  `BudgetSpec`; record-form artifact validation (`soulstream://` beside
  `file://`, `ArtifactRef()` parse helper); agent-only rule for the new
  blocks; pattern grammar (`@every` / `@at` / 6-field cron); duplicate
  refusals; delivery-class surface
- [x] T002 [P] Table tests for every new field, per-kind refusals,
  record-form artifact, nested unknown-field refusal (mirror the existing
  backend-key test), pattern grammar, agent-only rules, in
  `declaration/declaration_test.go`
- [x] T003 New package `artifact/`: `Fetch` (materialise topic, lineage
  tip via `FindArtefact`, `GetAttachment`, `VerifyDigest` — digest
  mismatch refuses) and `Resolver` (scratch-dir copy, exec bit for the
  runner), with embedded-server tests in `artifact/artifact_test.go`
- [x] T004 `wrap/config.go`: `WakeSet` (Mention/Topics/Schedules/
  Subjects), `HomeTopic`, `InstructionSource` on `Config`; validation
  (home topic required for non-record kinds); `wrap/wake.go`: `Wake`
  grows `Kind`, `Body` — zero values reproduce today's mention shape

**Checkpoint**: declaration parses/validates the full 0005 §2 surface;
existing suites green.

## Phase 3: User Story 1 — the record declares what wakes the agent (P1) 🎯 MVP

- [x] T005 [US1] Generalize `handleWake` in `wrap/wake.go`: body falls
  back to `Wake.Body` when the trigger is not in the view; self-report
  taps `[]{author}` only when the author is non-empty; prompt fill grows
  `KIND` and `INSTRUCTIONS` (materialised per wake via
  `Config.Instructions`, failure parks the wake loudly); seam order
  unchanged
- [x] T006 [US1] `wrap/sources.go`: topic source (live subscription on
  `SOULSTREAM.TOPICS.OPS.<path>` + materialise catch-up with
  outcome-existence pruning, self-authored ops excluded, declared types
  only), schedule source (reconcile registrations with
  `Nats-Schedule`/`-Target`/`-TTL`; ordered ephemeral DeliverAll consumer
  over the TICKS subjects; trigger = stream sequence decimal), subject
  source (plain subscribe; trigger = lowercase-hex SHA-256 of payload);
  `wrap/wrap.go` Run drives the declared set, mention-only byte-identical
  when `Wakes == nil`
- [x] T007 [US1] `wrap/instructions.go`: record-backed
  `InstructionSource` over `artifact.Fetch` (tip, digest-checked, in
  memory only); `wrap/declared.go`: `DeclaredConfig` mapping declaration
  → Config (wake set, home topic, budget block, instructions, prompt
  growth when the template lacks `{{INSTRUCTIONS}}`, persona match)
- [x] T008 [P] [US1] Unit tests: per-kind `handleWake` behavior (body
  fallback, tap rules, placement via `Wake.Topic`, instructions in
  prompt, materialisation failure parks), `DeclaredConfig` mapping and
  refusals, in `wrap/wake_test.go` + `wrap/declared_test.go`
- [x] T009 [US1] `cmd/soulstream-wrap/main.go`: `--declaration <file>` —
  parse, validate, require role=agent and persona == lane persona, apply
  `DeclaredConfig`; without the flag, today's paths untouched
- [x] T010 [US1] Integration `integration/declared_test.go`: four kinds
  fire from ONE declaration (harness-mock invoker); restart → stream
  count and attempt count both 1 per stream-backed trigger; subject
  publish while down → nothing after restart (SC-001, SC-002)

**Checkpoint**: design 0005 acceptance bar 1 measured green.

## Phase 4: User Story 3 — self-exclusion and the budget on every kind (P1)

- [x] T011 [US3] Integration: an op authored by the declared persona
  never wakes its topic wake (zero invocations, SC-004); an
  uncooperative two-declared-agent topic-wake cycle halts at the budget
  with loud op-less refusals (SC-005), in `integration/declared_test.go`

**Checkpoint**: the colony gate holds on the generalized engine.

## Phase 5: User Story 2 — instructions live in the record (P1)

- [x] T012 [US2] Integration: declared instructions v1 in the first
  wake's prompt; `topic.Revise` to v2; next wake on the same engine
  carries v2; nothing written outside run scratch (SC-003), in
  `integration/declared_test.go`

**Checkpoint**: revision-without-redeploy measured green.

## Phase 6: User Story 4 — the artifact can live in the record (P2)

- [x] T013 [US4] `runner/runner.go`: `ArtifactSource` seam (nil =
  file-only, today's path byte-identical); `soulstream://` resolved into
  the run scratch after `work.open` (failure → `work.abandon`); a
  record-form artifact with no source refuses pre-publish
- [x] T014 [P] [US4] Integration: launch a `soulstream://` declaration
  end-to-end (native backend, embedded realm); digest mismatch refuses;
  `file://` launch byte-identical, in `integration/declared_test.go` +
  runner unit tests

**Checkpoint**: registration-points-at-the-record boots.

## Phase 7: User Story 5 — capabilities are declared names (P3)

- [x] T015 [US5] Covered by T001/T002 validation; ensure spec's
  names-not-grants documentation on the fields and the
  `capability-minting` follow-on note in the package doc

## Phase 8: User Story 6 — nothing existing moves (P1)

- [x] T016 [US6] Schedule semantics measured live: re-publish replaces
  (no double tick rate), purge deregisters, `Nats-Schedule-TTL` bounds
  the backlog (SC-006) — a divergence STOPS the slice and is recorded in
  spec + report, in `integration/declared_test.go`
- [x] T017 [US6] Full back-compat sweep: existing suites unmodified
  except declaration-field enumeration (SC-007); tag-gated suites still
  compile (`go vet ./...`, `go build -tags msb_e2e|k8s_e2e|wrap_e2e`)

## Final Phase: Polish & Cross-Cutting

- [x] T018 Package docs grown (declaration, wrap, artifact) with the
  delivery classes, the one identity rule, and the runtime-side-reads
  standing (S3); `.specify/feature.json` → specs/009-agent-declaration
- [x] T019 Full gate: `make check` green; merge `--no-ff` to main as
  `Merge 009-agent-declaration: the record declares, the room runs`

## Dependencies & Execution Order

- Phase 2 (T001–T004) blocks everything; T003 blocks T007/T013.
- US1: T005→T006→T007 (same files touch), T008 after T005/T007, T009
  after T007, T010 after T006/T009.
- US3 (T011), US2 (T012), US6 (T016) after T006; US4 (T013–T014) only
  needs T001/T003.
- T017–T019 last.

## Implementation Strategy

Pure halves first (declaration schema, mapping), then the engine
generalization behind the existing seam order, then the sources, then the
measured integration evidence. The schedule slice verifies the
`Nats-Schedule-TTL` mechanism live before anything else builds on it
(the one design assumption core's gate did not directly measure).

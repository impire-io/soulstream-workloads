# Tasks: Loop safety — the wake budget

**Input**: Design documents from `/specs/008-loop-safety/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/library.md, quickstart.md

**Tests**: included — the repo's constitution makes the all-green gate and
measured acceptance criteria mandatory (S5; spec SC-001…SC-006), and the
research rig's discriminating cases are ported as this feature's tests.

**Organization**: tasks grouped by user story; US1 is the MVP.

## Phase 1: Setup

No project setup needed — the feature grows the existing `wrap` package.

## Phase 2: Foundational (blocking prerequisites)

- [x] T001 Grow wrap's local view type `Turn` with `Timestamp time.Time` and map it from the contribution in `agentRealm.Read`, in `wrap/wrap.go` (type lives in `wrap/correlate.go`); adjust existing fakes/tests that construct `Turn` only where compilation requires it
- [x] T002 Add `Budget` struct (`MaxHops`, `WindowMax`, `WindowPer`) and `Unbudgeted bool` to `Config`, defaults `{4, 8, 10m}` applied in `ApplyDefaults` exactly when `Unbudgeted` is false and `Budget` is wholly zero; negative values refused by validation with a teaching error, in `wrap/config.go`
- [x] T003 [P] Table-driven tests for defaults, partial budgets, opt-out, and negative-value refusal in `wrap/config_test.go`

**Checkpoint**: config surface and view carry everything the gate needs; all existing tests still green.

## Phase 3: User Story 1 — the runaway cycle halts without cooperation (P1) 🎯 MVP

**Goal**: an uncooperative agent-wakes-agent cascade halts at the pre-computed bound with loud, op-less refusals — including the id-evading (self-post) variant.

**Independent Test**: two wrapped script agents that always mention each other; one human root; cascade halts at exactly `MaxHops` outcomes (wrapper-posted) and within `2×WindowMax` (self-posted); every refusal is one `wake_refused` log line and zero ops.

- [x] T004 [US1] Implement the pure walker — `ParentOf` (with match count), `ChainToRoot` (with ambiguity count), `ProvableHops` — beside `WakeOpID` in `wrap/correlate.go`
- [x] T005 [US1] Implement `BudgetDecision(b, view, trigger, persona, now)` (depth part + window part + composition; zero parts disabled; legible reason with the numbers) in `wrap/correlate.go`
- [x] T006 [P] [US1] Table-driven walker tests — resolution to root, chain roots at human posts and at arbitrary-id posts, depth counting, ambiguity reported — in `wrap/correlate_test.go`
- [x] T007 [P] [US1] Table-driven `BudgetDecision` tests — depth refusal at the boundary (hops+1 > D), window refusal at K within W, window ignores ops older than W and other authors' ops, zero parts never refuse — in `wrap/correlate_test.go`
- [x] T008 [US1] Wire the gate into `handleWake` in `wrap/wake.go`: after the self-skip and outcome-existence pre-check, before the harness; on refusal log `wake_refused` (persona, topic, trigger op, reason) at Warn, post nothing, return outcome `"refused"`
- [x] T009 [US1] Unit tests on the existing fake-realm shape: a refused wake posts nothing and returns `"refused"`; an admitted wake is byte-identical to today; the gate is not evaluated when `Unbudgeted`, in `wrap/wake_test.go`
- [x] T010 [US1] Integration test (embedded server, real wrappers, script invokers — the rig's cases): uncooperative two-agent cycle halts at exactly `MaxHops` outcomes with ≥1 refusal and no refusal op; id-evading self-post cycle halts within `2×WindowMax`, in `integration/budget_test.go`

**Checkpoint**: SC-001, SC-002, SC-004 measured green — the colony gate's MVP stands.

## Phase 4: User Story 2 — legitimate delegation is untouched (P1)

**Goal**: the budget is a budget, not a ban.

**Independent Test**: human-rooted owner→A→B→A completes under defaults with zero refusals.

- [x] T011 [US2] Integration test: delegation chain of 3 outcomes completes under default budgets, zero refusals, every outcome walker-attributable to the human root; plus a single ordinary mention behaving exactly as today, in `integration/budget_test.go`

**Checkpoint**: SC-003 measured green.

## Phase 5: User Story 3 — the operator can see why (P2)

**Goal**: every refusal explainable from its one log line; cascades auditable from the record.

**Independent Test**: refusal log carries budget kind + numbers; walker output answers "who triggered what".

- [x] T012 [US3] Assert refusal-log content (which budget, hops vs D or count vs K/W) via a captured `slog` handler in `wrap/wake_test.go`; assert walker ambiguity surfaces to the caller (constructed collision case) in `wrap/correlate_test.go`

**Checkpoint**: SC-006 green.

## Phase 6: User Story 4 — opting out is explicit and byte-identical (P3)

**Goal**: `Unbudgeted: true` reproduces today's wrapper exactly, loudly once.

**Independent Test**: existing wake behavior unchanged under opt-out; one `wrap_unbudgeted` startup line.

- [x] T013 [US4] Log the unbudgeted standing once in `Run` (covers `Unbudgeted` and an explicit all-zero budget) in `wrap/wrap.go`, with a captured-handler test in `wrap/wake_test.go` or `wrap/config_test.go`
- [x] T014 [US4] Integration test: the US1 cascade scenario under `Unbudgeted: true` runs unbounded past the would-be bound within the run window (today's behavior), in `integration/budget_test.go`

**Checkpoint**: SC-005 green — the whole existing suite passes with opt-out semantics intact.

## Final Phase: Polish & Cross-Cutting

- [x] T015 Grow the `wrap` package doc (`wrap/wrap.go` doc comment) and the config commentary with the budget concept in the design's plain words (S3 duty; quickstart.md stays the consumer view)
- [x] T016 Full gate: `make fmt && make test && make lint` all green, `-race` on the integration suite; then the soul-hq journey episode for the build (`/journey-log`, same working session) and the design/roadmap "built" refreshes in soul-hq

## Dependencies & Execution Order

- Phase 2 (T001–T003) blocks everything.
- US1 (T004–T010): T004→T005→T008 sequential (same files); T006/T007 parallel with T008 after T005; T009 after T008; T010 after T008.
- US2 (T011) and US4 (T014) depend on T008/T010's harness shape; US3 (T012) on T008.
- T015–T016 last.

## Implementation Strategy

MVP is Phase 2 + US1 (the halt itself). US2–US4 are each one focused
test-plus-wiring increment on top. Everything rides the existing package
split: pure halves first (walker, decision), then the one wiring point,
then the integration ports of the rig's measured cases.

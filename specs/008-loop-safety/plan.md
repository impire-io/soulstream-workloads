# Implementation Plan: Loop safety — the wake budget

**Branch**: `008-loop-safety` | **Date**: 2026-08-25 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/008-loop-safety/spec.md`; soul-hq design [`0006-loop-safety.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0006-loop-safety.md) (graduated research, episode 0128 — all four bars measured)

## Summary

A composed wake budget enforced at wake admission in the wrap package: the
authorship-window floor (K own turn.posts per topic per W) and the
provable-chain depth bound (D hops over the `WakeOpID` UUIDv5 binding), both
computed from the topic view `handleWake` already reads. Refusals are
op-less and loud (`wake_refused`, one structured line with the numbers),
evaluated after the self-skip and outcome-existence pre-check, before the
harness. Zero budget = today's behavior byte-for-byte, plus one startup
line naming the unbudgeted standing. The mechanism was proven end-to-end by
the research rig (episode 0128); this build ports it into the shipped
package with the rig's discriminating cases as tests.

## Technical Context

**Language/Version**: Go 1.26 (repo standard)
**Primary Dependencies**: existing only — `soulstream-core` (realm/topic/record), `nats.go`, `google/uuid` (already the `WakeOpID` dependency); **no new dependencies**
**Storage**: none — the record is the only state (constitution soulstream-workloads I); budget knobs are wrapper configuration
**Testing**: `go test -race`; table-driven unit tests on the pure half (no NATS), embedded-JetStream integration tests on the shape already in `integration/wrap_test.go`
**Target Platform**: wherever wrap runs today (darwin/linux hosts)
**Project Type**: library package (`wrap`) inside the existing module
**Performance Goals**: admission adds only view-local computation; the depth walk is bounded by view size (research walker resolved 421-op views without measurable drag on the cascade — the rig sustained 84 wakes/s *with* materialise reads per wake)
**Constraints**: no core wire change; no new op types; refusal must not post; gate must not run when both knobs are zero
**Scale/Scope**: one package touched (`wrap`), two files grown (`correlate.go` pure half, `wake.go`/`config.go`/`wrap.go` wiring), one integration test file added

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **S1 NATS-Native First — PASS.** No new infrastructure; the budget reads
  the materialised topic view the wake already reads. Nothing beside NATS.
- **S2 Smallest Viable — PASS.** Three knobs, one gate function, one pure
  walker; no plugin points, no speculative config. The future dispatcher
  seam is *named* by design 0006 §6 but not built (its concrete occupant is
  design 0005's dispatcher, by demand).
- **S3 Docs First — PASS.** The wrap package doc and config doc grow the
  budget concept in the same change; design 0006 is the normative home.
- **S4 Research Gates — PASS.** The gate is episode 0128: all four bars
  measured before this build spends.
- **S5 All-Green — PASS.** `make fmt && make test && make lint`, `-race`
  suites hermetic on the embedded server.
- **soulstream-workloads I (Substrate Boundary) — PASS.** No durable truth
  in the runtime: budget state is *derived* from the record per wake, never
  stored.
- **soulstream-workloads II (One Identity) — PASS.** No human/machine
  branch: the budget applies to the wrapped persona's wakes regardless of
  who authored the trigger; authorship counting uses the same mechanical
  authorship every reader gets.
- **soulstream-workloads III (Contracts Orthogonal to Backends) — PASS.**
  wrap is host-side; no backend involvement.
- **soulstream-workloads V (Observable/Attributable) — PASS with a named
  shape.** A refusal is deliberately op-less (design 0006 §2: a refusal
  that posts is a wake source — the measured 312-turn failure ping-pong).
  Observability rides the structured log line, and the walker makes any
  cascade attributable from the record. The op-less choice is the design's
  explicit, measured decision, not a silent hole.

Post-design re-check: unchanged — PASS on all articles.

## Project Structure

### Documentation (this feature)

```text
specs/008-loop-safety/
├── spec.md              # /speckit-specify output
├── plan.md              # This file
├── research.md          # Phase 0: decisions carried from episode 0128
├── data-model.md        # Phase 1: Budget, hop, chain, refusal
├── quickstart.md        # Phase 1: consumer view
├── contracts/
│   └── library.md       # Phase 1: the wrap package's grown surface
├── checklists/
│   └── requirements.md  # spec quality checklist (done)
└── tasks.md             # Phase 2 (/speckit-tasks — not this command)
```

### Source Code (repository root)

```text
wrap/
├── config.go        # + Budget struct on Config; defaults in ApplyDefaults
├── correlate.go     # + the pure walker: ParentOf, ChainToRoot, ProvableHops
│                    # + the pure gate decisions: BudgetDecision over a view
├── correlate_test.go# + table-driven walker/gate tests (no NATS)
├── wake.go          # + the admission gate call in handleWake (after
│                    #   self-skip + outcome-existence, before invoke)
├── wake_test.go     # + refusal-path unit tests over the existing fakes
├── wrap.go          # + unbudgeted-standing startup log (once)
└── config_test.go   # + defaults/zero-budget tests

integration/
└── budget_test.go   # embedded-server: the rig's discriminating cases —
                     # uncooperative cycle halts at D; self-post cycle
                     # halts ≤ 2K; delegation completes with 0 refusals;
                     # zero-budget byte-identical
```

**Structure Decision**: grow the existing wrap package along its stated
split — `correlate.go` stays the pure, I/O-free half (the file's own
doc comment names this pattern), `wake.go` stays the orchestration half.
No new package: the walker and gate are wake-correlation logic, exactly
what correlate.go holds.

## Complexity Tracking

No constitution violations to justify.

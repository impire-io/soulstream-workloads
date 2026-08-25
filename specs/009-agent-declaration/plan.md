# Implementation Plan: Agent declaration — wake, instructions, capabilities

**Branch**: `009-agent-declaration` | **Date**: 2026-08-25 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/009-agent-declaration/spec.md`; soul-hq designs [`0005-agent-declaration.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0005-agent-declaration.md) (graduated, episode 0126) and [`0006-loop-safety.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0006-loop-safety.md) (built, specs/008); core f0a09f2's SOULSTREAM_SYSTEM stream (measured schedule mechanics).

## Summary

The declaration grows four optional agent-only blocks (`wake`,
`instructions`, `capabilities`, `budget`) and the record-form artifact;
the wrap engine generalizes its measured mention protocol into a wake
engine over four kinds, all through the one identity rule
(`WakeOpID(trigger, persona)`, frozen namespace) and the one admission
seam (self-skip → outcome-existence → 0006 budget → invoke → discharge).
Schedules ride the SOULSTREAM_SYSTEM stream's message scheduling (a
registration is one headered message; ticks TTL-bounded); subjects are
honestly at-most-once; instructions are materialised from the record at
every wake. Enforcement reads stay runtime-side (the resolved [O]);
capabilities are schema-only with `capability-minting` the named
follow-on. Back-compat is byte-for-byte: no declaration → mention-only
wrap exactly as today.

## Technical Context

**Language/Version**: Go 1.26 (repo standard)
**Primary Dependencies**: existing only — `soulstream-core` at the go.work
sibling (realm/topic/record; SystemScheduleSubject/SystemTickSubject,
FindArtefact/GetAttachment/VerifyDigest), `nats.go` v1.52.0
(ScheduleHeader/ScheduleTargetHeader/ScheduleTTLHeader), `google/uuid`;
**no new dependencies**, no go.mod pin bump (tagging is a human act)
**Storage**: none — the record is the only state (constitution
soulstream-workloads I); schedule registrations are declarative state the
substrate carries; no durable consumers, no dispatcher state
**Testing**: table-driven unit tests on the pure halves (declaration,
correlate, config mapping — no NATS); embedded-JetStream integration
tests on the `integration/` rig shapes (`client.Provision` gives
SOULSTREAM_SYSTEM); tag-gated suites (msb/k8s/wrap-live) still compile
**Target Platform**: wherever wrap runs today (darwin/linux hosts)
**Project Type**: library packages (`declaration`, `wrap`, `runner`, new
`artifact`) + the `soulstream-wrap` host cmd, inside the existing module
**Performance Goals**: wake admission stays view-local; schedule cadence
is server-side (~1.3 s for `@every 1s`, measured by core's gate); topic
catch-up is one materialise per declared path per catch-up
**Constraints**: no core wire change; no new op types; frozen WakeOpID
namespace; agent minter scope not widened; refusals op-less; `file://`
and mention-only paths byte-identical
**Scale/Scope**: four packages touched, one added; one host flag; one
integration test file added; existing tests unmodified except
declaration-field enumeration

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **S1 NATS-Native First — PASS.** The one additive stream already exists
  (core 020); schedules are JetStream message scheduling, subjects are
  core NATS, positions are the record. Nothing beside NATS.
- **S2 Smallest Viable — PASS.** Growth is schema + one engine
  generalization; no plugin registry, no dispatcher, no identity
  dependency. The artifact-source seam and InstructionSource are each one
  interface with one shipped occupant.
- **S3 Docs First — PASS.** Designs 0005/0006 are the normative homes;
  the package docs and this spec's contract carry the delivery classes
  and trigger identities in the same change.
- **S4 Research Gates — PASS.** Episode 0126 measured the mechanisms
  (record-form boot, instructions tip, capability tags, schedule ticks);
  episode 0128's budget is already built (specs/008). The one
  under-measured mechanism (Nats-Schedule-TTL stamping ticks) is verified
  live by this feature's tests before anything builds on it.
- **S5 All-Green — PASS.** `make check` before every commit; hermetic
  suites, zero skips; tag-gated suites compile.
- **soulstream-workloads I (Substrate Boundary) — PASS.** No durable
  truth in the runtime: positions are outcome ops, schedules are
  substrate state, instructions/artifacts are materialised per use and
  never cached durably.
- **soulstream-workloads II (One Identity) — PASS.** A declared agent is
  a persona like every other; no human/machine branch; self-exclusion is
  mechanical authorship, not identity kind.
- **soulstream-workloads III (Contracts Orthogonal to Backends) — PASS.**
  The declaration still names no backend; the record-form artifact
  resolves to a local path behind the runner's seam, so every backend
  keeps its existing LaunchSpec shape.
- **soulstream-workloads V (Observable/Attributable) — PASS with the 008
  shape.** Refusals stay op-less by measured design; every outcome is an
  attributed op under a deterministic id; ticks/subject messages are
  declared non-authoritative plumbing.

Post-design re-check: unchanged — PASS on all articles.

## Project Structure

### Documentation (this feature)

```text
specs/009-agent-declaration/
├── spec.md              # /speckit-specify output (fixed decisions recorded)
├── plan.md              # This file
├── contracts/
│   └── wake-kinds.md    # trigger identities, delivery classes, placement,
│                        # self-report rules, runtime-side reads, schedule wire
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
declaration/
├── declaration.go       # + Instructions/Capabilities/Wake/Budget fields,
│                        #   record-form artifact, per-kind validation,
│                        #   pattern grammar, delivery classes
└── declaration_test.go  # + table tests per field/kind/refusal

artifact/                # NEW: record-form artefact fetch (NATS-touching)
├── artifact.go          # Fetch (tip, digest-checked) + Resolver (scratch copy)
└── artifact_test.go     # embedded-server fetch/digest tests

runner/
└── runner.go            # + ArtifactSource seam; soulstream:// resolved into
                         #   the run scratch after work.open, refusal → abandon

wrap/
├── wake.go              # + Wake{Kind, Body}; per-kind body/taps/placement in
│                        #   handleWake (same seam order)
├── config.go            # + WakeSet (Topics/Schedules/Subjects/Mention),
│                        #   HomeTopic, InstructionSource; validation
├── sources.go           # NEW: topic/schedule/subject sources + reconcile
├── instructions.go      # NEW: record-backed InstructionSource (engine reads)
├── declared.go          # NEW: DeclaredConfig — declaration → engine config
├── wrap.go              # Run drives the declared source set; mention-only
│                        #   path byte-identical when no WakeSet
└── *_test.go            # + per-kind handleWake units, mapping units

cmd/soulstream-wrap/
└── main.go              # + --declaration flag (persona must match lane)

integration/
└── declared_test.go     # NEW: four kinds exactly-once across restart;
                         # self-exclusion; topic-cycle budget halt;
                         # instructions revision; schedule replace/TTL;
                         # subject loss-when-down
```

**Structure Decision**: the engine generalization stays inside `wrap`
(the dispatcher of design 0005 §9 will consume the same package — the
admission seam is inherited, per 0006 §6). Record-artefact fetching gets
its own small NATS-touching package (`artifact`) because two consumers
need it (runner's resolver, wrap's instructions) and `declaration` must
stay pure. `declaration` keeps zero NATS imports; `wrap` may import
`declaration` (no cycle — declaration imports nothing of wrap).

## Complexity Tracking

No constitution violations to justify.

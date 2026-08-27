# Implementation Plan: Capability minting — the scope carries the selectors

**Branch**: `010-capability-minting` | **Date**: 2026-08-27 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification; soul-hq designs `0005-agent-declaration.md` §5 (graduated, measured episode 0126 bar 3) and `0003-fleet.md` §5 (identity minting path preferred).

## Summary

The declaration's `capabilities` block (schema-only since 009) reaches the
minter seam: `minter.Scope` gains the capabilities selector, `runner.Launch`
passes it through, the local `SigningKeyMinter` honors it by narrowing the
tool-namespace wildcard to the declared tools, and the scope renders its
mint tags (`persona:`/`topic:`/`tool:`) in one exported, refusing surface
for the external minter the product wires. No identity-plane dependency —
the seam substitution stays the consumer's act (design 0001 §4).

## Technical Context

**Language/Version**: Go 1.26 (repo standard)
**Primary Dependencies**: existing only — `soulstream-core` (identity name
grammar, topic subjects), `nats-io/jwt/v2`, `nkeys`; **no new dependencies**
**Storage**: none — permissions live in the minted user JWT
**Testing**: golden units on the pure scope derivation; the operator-mode
`integration/` rig for transport enforcement (SC-001/002); fleet round-trip
unit
**Target Platform**: unchanged
**Project Type**: library packages (`minter`, `declaration`, `runner`) in the
existing module
**Performance Goals**: none new — derivation stays pure
**Constraints**: capability-less scopes byte-identical; no identity import
(cycle guard); tag values never alter subject grammar
**Scale/Scope**: three packages touched, ~4 files + tests

## Constitution Check

- **S1 NATS-Native First — PASS.** Permissions are NATS user-JWT allow
  lists; tags are NATS user-claim tags. Nothing beside NATS.
- **S2 Smallest Viable — PASS.** One field on Scope, one narrowing rule,
  one tag renderer; no identity dependency, no new interface.
- **S3 Docs First — PASS.** Design 0005 §5 is the normative home; package
  docs updated in the same change.
- **S4 Research Gates — PASS.** Episode 0126 bar 3 measured the mechanism
  (tag-template mint, transport refusal, zero deliveries); this build's
  local narrowing reproduces the same observable hermetically.
- **S5 All-Green — PASS.** `make check` before every commit.
- **soulstream-workloads I (Substrate Boundary) — PASS.** No new state.
- **soulstream-workloads II (One Identity) — PASS.** Capabilities narrow a
  persona's credential; no identity kind appears.
- **soulstream-workloads III (Contracts Orthogonal to Backends) — PASS.**
  Backends see the same `LaunchSpec.Cred`; nothing backend-visible changes.
- **soulstream-workloads V (Observable/Attributable) — PASS.** Mint
  refusals are loud preflight errors before any op publishes (FR-008
  discipline of 001, unchanged).

## Project Structure

### Documentation (this feature)

```text
specs/010-capability-minting/
├── spec.md              # fixed decisions recorded
├── plan.md              # this file
└── tasks.md             # execution checklist
```

### Source Code (repository root)

```text
declaration/
├── declaration.go       # Capabilities.Validate exported (one grammar source);
│                        #   ValidateTopicPath exported for the same reason
└── declaration_test.go  # + exported-surface coverage

minter/
├── scope.go             # + Scope.Capabilities, narrowing, tag vocabulary,
│                        #   MintTags, capability validation
├── minter.go            # Minter contract doc (honor-or-refuse), Mint
│                        #   re-validates capabilities
└── scope_test.go        # golden byte-identical lists, narrowing, tags,
│                        #   refusals

runner/
└── runner.go            # Launch passes d.Capabilities into the scope

fleet/
└── fleet_test.go        # capability declaration round-trips Submit/projection

integration/
└── scope_test.go        # + TestCapabilityScopeEnforced (SC-001/002)
```

**Structure Decision**: the tag vocabulary lives in `minter` (the scope is
the value source and the local minter needs the same grammar guard); the
grammar itself stays single-source in `declaration`/core-`identity`,
reached via the newly exported `Capabilities.Validate` and
`ValidateTopicPath` (FR-008).

## Complexity Tracking

No constitution violations to justify.

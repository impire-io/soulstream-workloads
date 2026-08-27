# Feature Specification: Capability minting — the scope carries the selectors

**Feature Branch**: `010-capability-minting`
**Created**: 2026-08-27
**Status**: Draft
**Input**: soul-hq design [`0005-agent-declaration.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0005-agent-declaration.md) §5 (capability enforcement — the identity plane, unchanged, measured in research episode 0126 bar 3) and [`0003-fleet.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0003-fleet.md) §5 (per-workload minting, identity path preferred). This is the workloads half of the named follow-on `capability-minting`: the declaration's `capabilities` block (schema-only since specs/009) reaches the minter seam. The identity-backed Minter itself lives in the product repo — this repo deliberately keeps **no identity-plane dependency** (the cycle guard; consumers wire the seam).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A declared agent reaches exactly its granted tools (Priority: P1)

An operator declares an agent with `capabilities: {role: "agent", tools: ["search"]}`. The launched workload's credential lets it call `search` and nothing else on the tool namespace — the ungranted call dies at the transport, with zero authorization code in the runtime (design 0005 §10 acceptance 3).

**Why this priority**: This is the feature — capabilities stop being decoration.

**Independent Test**: Against the operator-mode rig, a credential minted for a capability-bearing scope answers on the granted tool's service subject and draws a server-side permissions violation on any other, with zero deliveries to the ungranted responder.

**Acceptance Scenarios**:

1. **Given** an agent scope declaring tools `[toola]`, **When** its minted credential calls `SOULSTREAM.SVC.toola`, **Then** the granted tool answers; **When** it publishes to `SOULSTREAM.SVC.toolb`, **Then** the server refuses with a permissions violation and the `toolb` responder receives nothing.
2. **Given** an agent scope declaring `tools: []` (role only), **When** its credential connects, **Then** the connection admits and every tool-namespace publish refuses.

---

### User Story 2 - A capability-less declaration is byte-identical to today (Priority: P1)

Every existing declaration — no `capabilities` block — mints exactly the credential it minted yesterday, permission list byte-for-byte.

**Why this priority**: Compatibility floor; capability-minting must be additive.

**Independent Test**: Golden unit — `Scope{Capabilities: nil}` permission lists equal today's lists exactly, agent and tool roles both.

**Acceptance Scenarios**:

1. **Given** a scope without capabilities, **When** permissions derive, **Then** the pub/sub lists are byte-identical to the pre-feature lists (agents keep the `SOULSTREAM.SVC.>` wildcard).

---

### User Story 3 - The mint-tag vocabulary is one refusing surface (Priority: P2)

The external (identity-backed) minter needs the scope rendered as mint tags (`persona:<p>`, `topic:<t>`, `tool:<n>`), and a hostile or corrupted value must never become a subject-grammar injection (`tool:">"` would widen a template to everything).

**Why this priority**: The tag list is the cross-plane contract the product's minter sends to the identity plane; it must be constructed in exactly one place and refuse bad values there.

**Independent Test**: Unit — `MintTags` renders the canonical list for a capability scope, returns nothing for a capability-less one, and refuses any value that is not valid by the name/path grammar.

**Acceptance Scenarios**:

1. **Given** a scope with persona `sprite`, topic `acme.q2-ab12`, tools `[toola, toolb]`, **When** tags render, **Then** the list is exactly `["persona:sprite", "topic:acme.q2-ab12", "tool:toola", "tool:toolb"]`.
2. **Given** a tool name (or persona, or topic segment) containing a subject metacharacter, **When** tags render or permissions derive through `Mint`, **Then** the mint refuses loudly.

---

### Edge Cases

- Capabilities on a tool-role scope refuse at mint (declarations already refuse this; the minter is a public surface and must not rely on it).
- A mint failure in `Launch` is a preflight refusal: it happens before `work.open`, so nothing publishes — no dangling claim, no new abandon machinery (the FR-008 discipline, unchanged).
- Fleet placement round-trips the whole declaration as JSON; a capability-bearing declaration must survive `Submit` → projection → launch unchanged.
- Duplicate tools are refused at declaration validation; the minter's own validation re-refuses them (defense in depth on a public API).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `minter.Scope` MUST carry the declaration's capabilities (`*declaration.Capabilities`); `runner.Launch` MUST pass them through. Absent capabilities MUST derive permission lists byte-identical to today's.
- **FR-002**: For an agent scope with capabilities, the derived permissions MUST replace the tool-namespace wildcard with exactly one entry per declared tool (`SOULSTREAM.SVC.<tool>`); an empty tool list MUST grant no tool-namespace entry at all.
- **FR-003**: The package MUST render the scope's mint tags in one exported surface (`MintTags`): `persona:<persona>`, `topic:<topic-path>`, then `tool:<name>` per declared tool, in declaration order; a capability-less scope renders no tags.
- **FR-004**: Tag rendering and minting MUST refuse any persona, topic segment, or tool name that fails the shared name/path grammar — never emit a value that could alter subject grammar.
- **FR-005**: The `Minter` contract MUST state that implementations honor `Scope.Capabilities` (narrow or refuse — never ignore); `SigningKeyMinter` honors by narrowing (FR-002).
- **FR-006**: `SigningKeyMinter.Mint` MUST refuse capabilities on a non-agent role and MUST re-validate capability values (grammar + duplicates) before deriving permissions.
- **FR-007**: This repo MUST NOT gain an identity-plane dependency; the identity-backed Minter is the consumer's (the product's) to wire through the existing seam.
- **FR-008**: Declaration validation (`Capabilities`) and topic-path validation MUST be reachable by the minter from their single source — no duplicated grammar.

### Key Entities

- **Capability scope**: a `minter.Scope` carrying `Capabilities {role, tools[]}` — selectors, never grants.
- **Mint tag**: one `key:value` string in the identity plane's tag vocabulary (`tool`, `topic`, `persona`); inert without an account template that resolves it.
- **The narrowing**: the local minter's honoring of capabilities — declared tools replace the tool-namespace wildcard.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Against the operator-mode rig, a capability credential's granted tool answers and an ungranted publish draws a server-side permissions violation with zero deliveries to the ungranted responder.
- **SC-002**: A role-only (zero-tool) capability credential admits and reaches no tool subject.
- **SC-003**: Capability-less scopes derive permission lists byte-identical to the pre-feature lists (golden unit), and the entire existing suite passes unchanged.
- **SC-004**: A capability-bearing declaration survives the fleet submission round-trip intact.
- **SC-005**: `make check` green — fmt, tidy, build, test, lint; tag-gated suites still compile.

## Assumptions

- The account-template half (`SOULSTREAM.SVC.{{tag(tool)}}` and siblings) is the identity plane's exported surface, built in its own repo; the two halves are drift-courted by the product's consumer-position e2e (the cycle guard makes a shared constant impossible by construction).
- Tag keys `tool`/`topic`/`persona` are the design 0005 §5 / fleet 0003 §5 vocabulary, dual-written on the identity side by necessity.
- NATS lowercases tags at claim level; the shared name grammar is already lowercase-only, so no value changes shape in transit.

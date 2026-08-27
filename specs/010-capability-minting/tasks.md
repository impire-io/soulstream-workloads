# Tasks: Capability minting — the scope carries the selectors

**Input**: spec.md, plan.md in this directory.

- [x] T001 Export the single grammar sources: `declaration.Capabilities.Validate`
      (was `validate`) and `declaration.ValidateTopicPath` (was
      `validateTopicPath`); call sites updated; declaration tests extended.
- [x] T002 `minter.Scope` gains `Capabilities *declaration.Capabilities`;
      `PermissionSet` narrows the agent's `SOULSTREAM.SVC.>` wildcard to one
      entry per declared tool (empty list → no tool-namespace entry);
      capability-less derivation byte-identical.
- [x] T003 Tag vocabulary in `minter/scope.go`: `TagKeyTool/Topic/Persona`,
      `MintTags` (canonical order, grammar-refusing, nil for capability-less
      scopes).
- [x] T004 `SigningKeyMinter.Mint` re-validates capabilities (agent-only,
      grammar, duplicates) before deriving permissions; `Minter` interface doc
      carries the honor-or-refuse contract.
- [x] T005 `runner.Launch` passes `d.Capabilities` into the scope (mint stays
      before `work.open` — preflight refusal discipline unchanged).
- [x] T006 Golden units in `minter/scope_test.go`: byte-identical
      capability-less lists (agent + tool), narrowing, zero-tool case, tag
      rendering, injection refusals, duplicate refusal.
- [x] T007 Fleet round-trip unit: a capability-bearing declaration survives
      `Submit` → projection intact.
- [x] T008 `integration/scope_test.go` + `TestCapabilityScopeEnforced`:
      granted tool answers; ungranted publish refused server-side with zero
      deliveries to its responder; zero-tool credential admits and reaches no
      tool subject (SC-001/002).
- [x] T009 `make check` green; tag-gated suites compile.

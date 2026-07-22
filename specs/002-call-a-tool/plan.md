# Implementation Plan: Call a tool

**Branch**: `002-call-a-tool` | **Date**: 2026-07-22 | **Spec**: [`spec.md`](spec.md)

## Summary

Extend M1.1 (not replace it) with the `tool` role and persistent-service
support. Three focused changes plus a reference tool: the declaration accepts
`tool`, the minter grows **role-aware scopes**, and the runner learns to
**launch and stop** a persistent workload (not only run one to completion).
Discovery-and-call is soulstream's own `SOULSTREAM.SVC.*` request-reply — a tool
serves on `SOULSTREAM.SVC.<persona>`; a caller derives that subject from the
tool's name and requests it (no-responders = not found). No new dependency, no
`$SRV`/registry.

## Constitution Check

Against [`hq/00-GENESIS/constitution.md`](../../hq/00-GENESIS/constitution.md):

- **I. Substrate boundary** — PASS. Still no store of record; tool lifecycle is
  work ops on the topic, scratch reaped.
- **II. One identity, no privileged tier** — PASS. The tool and the caller are
  peers, each with a scoped credential (serve-scope / call-scope); a tool is a
  callable capability, not an API tier.
- **III. Contracts orthogonal to backends** — PASS. `tool` runs under the same
  native backend, same declaration shape (no backend field).
- **IV / V / VI** — PASS. Post-gate; lifecycle legible as ops (launch → stop);
  gate stays green with pure logic server-free.

No violations.

## Key decisions

1. **Role-aware scopes** (`minter`). `Scope` gains a `Role`. An **agent** keeps
   its topic scope plus the ability to *call* tools (pub `SOULSTREAM.SVC.>`). A
   **tool** gets a *serve* scope only: sub `SOULSTREAM.SVC.<persona>`, pub
   `_INBOX.>` (replies). Nothing else — SC-003 (§spec) enforces it.
2. **A tool's name is its persona**; its service subject is
   `SOULSTREAM.SVC.<persona>`. Discovery-by-name derives the subject; a request
   with no responders means "not found." No new declaration field.
3. **Runner launch/stop** (`runner`). `Launch` does preflight → `work.open` →
   `backend.Start` → `work.claim`, returning a `Running` handle. `Running.Wait`
   awaits self-exit and publishes the terminal op (M1.1 job/agent path);
   `Running.Stop` signals, reaps, and publishes `work.done` (intentional stop);
   `Running.Serve` is the CLI helper (self-exit → terminal, ctx-cancel → Stop).
   `Runner.Run` becomes `Launch` + `Wait` (unchanged M1.1 behaviour).
4. **Reference tool** `cmd/tool-upper`: subscribes `SOULSTREAM.SVC.<persona>`,
   replies upper-cased. The M1.2 analogue of `agent-echo`.

## Project structure (delta over M1.1)

```text
declaration/   # + accept role=tool, lifecycle=service
minter/        # + Role on Scope; role-aware PermissionSet (agent-call, tool-serve)
runner/        # + Launch/Running/Wait/Stop/Serve; Run = Launch+Wait
cmd/tool-upper/ # NEW reference tool
integration/   # + an agent discovers+calls a launched tool; scope enforcement
```

## Success criteria → verification

- SC-001 discover+call → integration test (uppercase round-trip).
- SC-002 launch/stop lifecycle ops → integration (open/claim on launch, done on stop).
- SC-003 both scopes enforced → operator-mode test (extend the M1.1 harness).
- SC-004 stop/crash reaping → runner unit tests + integration.

# Tasks: The standing dispatcher — agents as infrastructure

**Input**: Design documents from `/specs/011-dispatcher/`
**Prerequisites**: plan.md, spec.md

**Tests**: included — the constitution's all-green gate (S5) plus design
0007's acceptance bars 1–3 as measured integration evidence (bars 4–5
belong to the held inference work).

**Organization**: tasks grouped by user story; US1 (submit and forget) is
the MVP, US2 (two nodes) and US3 (the budget) ride the same loop.

## Phase 1: Setup

No project setup needed — one new package inside the existing module, no
new dependencies.

## Phase 2: Foundational (blocking prerequisites)

- [ ] T001 `fleet/fleet.go`: export the placement-body reader
  (`placementOf` → `DeclarationOf`), documented as the one seam a
  dispatcher owning its own claim path needs. No behavior change, no
  launch hook, `TryPlace`/`Sweep`/`Release` untouched.
- [ ] T002 New package `dispatcher/`: the `Dispatcher` value — node id,
  node client, placement topic, `ConnectAgent` hook, base engine config,
  invoker, the four cadences (reclaim, probe, sweep, poll), race
  backoff, logger — with `Validate`-at-`Run` startup refusals (missing
  node/client/topic/hook, a base config carrying a fixed persona).

**Checkpoint**: the package compiles, existing suites green.

## Phase 3: User Story 1 — submit and forget (P1) 🎯 MVP

- [ ] T003 [US1] The watch: live subscription on
  `topic.OpsSubject(placement topic)` poking a coalescing scan channel,
  plus a poll ticker as catch-up (design 0007 §9's build requirement —
  the spike's poll is the fallback, never the mechanism).
- [ ] T004 [US1] The scan: one materialise, then **resume** (claimed +
  owner == self + not yet served → serve, no op) and **race** (open +
  engine-servable + not backed off → claim path).
- [ ] T005 [US1] The claim path the dispatcher owns: `ClaimWork` →
  re-materialise → serve only if the read-back names this node owner;
  a won-but-unservable placement is abandoned back with a loud line and
  a bounded node-local backoff.
- [ ] T006 [US1] Serve: `wrap.DeclaredConfig(base, decl, client)` over
  the `ConnectAgent` client, run as a `wrap.Wrapper` in its own
  goroutine with its own cancel; per-placement scratch; persona-busy
  self-selection guard.
- [ ] T007 [US1] `Drain(ctx)`: stop claiming, cancel every engine, wait
  for each (bounded by the drain context), idempotent; `Run` drains when
  its context ends. Documented: hard stop is the process dying.
- [ ] T008 [P] [US1] Unit tests in `dispatcher/dispatcher_test.go`:
  startup refusals, the servable predicate across role/wake shapes,
  defaults, backoff arithmetic, `Drain` before `Run` and twice.
- [ ] T009 [US1] `cmd/soulstream-workloads/main.go`: `dispatcher serve`
  — node-side env only, per-persona creds-directory `ConnectAgent`,
  preset-or-template harness config, drain on SIGINT/SIGTERM; usage
  string grows the second verb.
- [ ] T010 [US1] Integration (SC-001, SC-002, SC-003) in
  `integration/dispatcher_test.go`: submitter closed → mention answered
  exactly once; restart resumes with **no new op** and no duplicate;
  connections dropped mid-run → zero outcomes, restart serves exactly
  once.

**Checkpoint**: design 0007 acceptance bar 1 measured green.

## Phase 4: User Story 2 — two nodes and failover (P1)

- [ ] T011 [US2] Liveness: answer `fleet.ProbeSubject(self)` for the
  served set (mutex-guarded — the callback and the serve/drain paths are
  different goroutines) with `fleet`'s wire answers; sweep on a cadence
  with a `Runner`-less `fleet.Node`.
- [ ] T012 [US2] Integration (SC-004, SC-005, SC-006): contested
  placements each one owner / one live claim; kill a node → survivor's
  timeline `claim,abandon,claim`; a wake posted in the failover window
  answered exactly once; zero probe traffic on the stream.

**Checkpoint**: design 0007 acceptance bar 2 measured green.

## Phase 5: User Story 3 — the budget rides the dispatcher path (P1)

- [ ] T013 [US3] Integration (SC-007): a declared budget halts the
  uncooperative two-agent cycle at its bound through the dispatcher,
  op-lessly and loudly; the legitimate owner→A→B→A delegation completes
  with zero refusals under defaults. No dispatcher-side admission code —
  the assertion is that the engine's seam stays reachable.

**Checkpoint**: design 0006 §6's inheritance rule measured, not asserted.

## Phase 6: User Story 4 — the runner path is not disturbed (P2)

- [ ] T014 [US4] Integration (SC-008): a `role: tool` placement and an
  agent placement with no wake set are left open with empty timelines
  while a dispatcher runs beside them.

## Final Phase: Polish & Cross-Cutting

- [ ] T015 Package doc for `dispatcher` carrying the loop, the resolved
  serve seam, the credential-hook boundary, and the drain/crash
  distinction (S3); `.specify/feature.json` → specs/011-dispatcher.
- [ ] T016 Full gate: `make check` green; the new integration cases run
  `-race -count=3` clean; back-compat sweep (`go vet ./...`, tag-gated
  suites still build).

## Dependencies & Execution Order

- T001–T002 block everything.
- US1: T003→T004→T005→T006→T007 (one file); T008 after T007; T009 after
  T006; T010 after T009.
- US2: T011 after T006 (the served set is what a probe answers);
  T012 after T011.
- US3 (T013) and US4 (T014) after T010.
- T015–T016 last.

## Implementation Strategy

The loop first as one readable file — watch, scan, claim, serve, drain —
then the liveness halves bolted beside it, then the acceptance bars as
standing tests in the order the design lists them. Each bar is written
against the shipped mechanisms only: if a test needs a new seam, that is
a finding about the design, not a licence to widen `fleet`.

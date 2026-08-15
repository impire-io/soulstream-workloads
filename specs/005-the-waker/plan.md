# Implementation Plan: The waker — notify-triggered invocation

**Branch**: `005-the-waker` | **Date**: 2026-08-15 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/005-the-waker/spec.md`

## Summary

A standing `waker serve` arm of the existing binary: one durable
AckExplicit consumer per registered agent on `SOULSTREAM_NOTIFY`, an
admission probe per wake, harness invocation through pure-configuration
templates, and a runner-owned reply obligation — every admitted wake ends
in exactly one outcome op, idempotent across redeliveries. All mechanics
were measured in the graduated research (episode 0082); this plan turns
the spike into the workload plane's trigger arm along the repo's existing
seams (narrow injected interfaces, pure classification, hermetic tests on
in-process NATS).

## Technical Context

**Language/Version**: Go 1.24 (repo standard)
**Primary Dependencies**: `soulstream-core` **v0.8.3** (bump from v0.8.0 —
prerequisite, see Complexity Tracking: v0.8.2's `realm.Config`
URL/CredsFile/Token lane + `PostTurnMentioning`, plus the new
`PostTurnIdempotent` from research D7); `nats.go/jetstream` (first direct
consumer code in this module); `soulstream-identity/client` v0.2.0 (new
edge — ephemeral lane only, behind a narrow interface, wired in `cmd/`)
**Storage**: none — registrations are operator config files, run dirs are
scratch, outcomes are ops (constitution workloads-I)
**Testing**: stdlib only; `internal/natstest.StartJetStream` for hermetic
protocol proofs, `StartOperator` for credential-enforcement claims;
call-sequence fakes in the `runner_test.go` style
**Target Platform**: darwin/linux daemon beside the runner
**Project Type**: single Go module extension (new `waker/` package + a
second noun in `cmd/soulstream-workloads`)
**Performance Goals**: none stated — wake latency is dominated by the
harness run (measured 4–12s); the waker adds milliseconds
**Constraints**: default gate hermetic (no external harness, server, or
login state); `AckWait` > run budget; notify backlog bounded by the
protocol at `realm.InboxWindow` (100/persona) — named, not hidden
**Scale/Scope**: one realm, one waker process, tens of registered agents

## Constitution Check

*GATE: passed pre-research; re-checked post-design — no violations.*

- **workloads I — Substrate boundary**: PASS. The waker stores nothing
  durable: registrations are versioned operator configuration, run
  directories are scratch deleted on reap, consumer positions belong to
  the transport, and every durable fact the waker produces is an op in a
  topic (spec FR-010, FR-012).
- **workloads II — One identity, no privileged tier**: PASS, argued. The
  *waker* is plane machinery — the exact standing the runner already has
  (the runner holds the realm signing seed via `minter`; the waker holds
  a durable consumer and, in the ephemeral lane, mint rights). No
  *workload* gains standing: every harness run sees only its agent's
  scoped credential, delivered per-run, and the research measured that no
  workload-minted scope could even create the waker's consumer
  (`$JS.API.INFO` is an agent's whole JetStream surface). Behaviour never
  branches on human vs machine — a woken agent is an ordinary persona,
  probed and refused by the same admission any client gets.
- **workloads III — Contracts orthogonal to backends**: PASS by
  abstention plus one analog. The backend seam and the declaration are
  untouched (research D1, D2 — wakes are waker-supervised subprocesses
  day one; the seam integration has a named trigger). The template plays
  III's role on the harness axis: waker code is harness-agnostic, proven
  by SC-004's byte-identical-binary criterion.
- **workloads IV — Research gates**: PASS. `agent-participation`
  graduated with four bars measured (episode 0082); design 0004 is the
  argument to this spec; research.md D1–D8 decide the remaining opens on
  that evidence.
- **workloads V — Observable, attributable execution**: PASS. Every
  admitted wake ends in an attributed op; failure testimony carries the
  waker's own authorship (never ghostwritten — research G2, forced by the
  measured 2ms revocation); refused wakes are deliberately op-less and
  therefore surfaced in the waker's log instead (research D8's slog
  decision exists exactly for this named gap).
- **workloads VI / S5 — All-green gate**: PASS by design. Hermetic
  default suite (scripted harnesses from `cmd/`, in-process NATS, no
  skips); the real-harness proof is opt-in (`make test-wake`), the
  M1.3/M2.1 pattern.
- **S1 — NATS-native**: PASS. Durable consumer, `Nats-Msg-Id` dedupe,
  and the notify stream are all built-in primitives; no queue, poller,
  or database appears. **Minimum server version**: none beyond the
  repo's existing floor — every primitive used (durable consumers,
  duplicate window, per-subject limits) predates NATS 2.12.
- **S2 — Smallest viable**: PASS. Two credential lanes exist because the
  spec (from measured research) names both; each hides behind a narrow
  interface with its concrete occupant. No plugin points: the template
  is data, not an extension API; declaration vocabulary deliberately
  deferred (D2) rather than speculatively added.
- **S3 — Documentation**: the feature ships with the README's waker
  section and the quickstart; the design doc (0004) is propagated on
  landing per how-we-work.

## Project Structure

### Documentation (this feature)

```text
specs/005-the-waker/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0: consolidated measurements + D1–D8
├── data-model.md        # Phase 1: registration/template/wake states
├── quickstart.md        # Phase 1: run a wake by hand (mock, then claude)
├── contracts/
│   └── waker-contract.md  # Registration file schema + wake-protocol invariants
├── checklists/requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
waker/
├── waker.go             # package doc (why the trigger arm exists, in
│                        #   constitutional terms) + Waker: Serve(ctx) loop,
│                        #   one consumer goroutine per registration
├── wake.go              # the per-delivery protocol: redelivery pre-check,
│                        #   admission probe, context materialisation,
│                        #   invoke, discharge (ack strictly after outcome)
├── registration.go      # Registration / Template / TerminalMap; config
│                        #   load + validation (template without terminal
│                        #   mapping refused at load — FR-004)
├── harness.go           # subprocess execution: fresh run dir, generated
│                        #   MCP config, sanitized env, process-group kill,
│                        #   dot-path terminal-event extraction
├── correlate.go         # PURE: wake op-id (UUIDv5 of notify op-id),
│                        #   before/after set difference, outcome
│                        #   classification (the lifecycle.go analog)
├── waker_test.go        # call-sequence fakes: "probe,invoke,post,ack" vs
│   wake_test.go         #   "probe,nak"; redelivery pre-check; idempotency
│   registration_test.go #   and refusal classes — no server
│   harness_test.go
│   correlate_test.go
cmd/soulstream-workloads/
├── main.go              # dispatch switch: `workload start` | `waker serve`;
│                        #   wakerConfigFromEnv() in the selectBackend style
│   main_test.go         #   (pure env→config functions, unit-tested)
cmd/harness-mock/
└── main.go              # scripted harness (reference-workload style):
                         #   emits claude-shaped or codex-shaped grammar by
                         #   flag; sleep/hang/die/self-post modes for faults
integration/
└── waker_test.go        # hermetic e2e: StartJetStream + Provision +
                         #   StartTopic + waker + harness-mock — SC-001/2/3/4
                         #   scenarios incl. both grammars, faults, backlog;
                         #   probe-refusal class against StartOperator creds
Makefile                 # + test-wake opt-in target (real `claude -p`)
go.mod                   # core → v0.8.3; + soulstream-identity/client
```

**Structure Decision**: `waker/` joins `runner/` as a sibling — the
runner is the *launch* arm, the waker the *trigger* arm, both thin
orchestrations over injected narrow interfaces with their pure logic in
a separate file (`lifecycle.go` ⇄ `correlate.go`). The second noun lands
in the existing binary (`soulstream-workloads waker serve`) rather than a new
`cmd/` dir: one operator install, one env vocabulary, dispatch by a
small switch, env→config mapping kept in pure unit-tested functions.
`harness-mock` follows the reference-workload category (`agent-echo`,
`tool-upper`): a single-file main the integration tests build with the
existing `buildCmd` helper.

## Design outline (how the pieces satisfy the spec)

- **FR-001/FR-002 (durable consumption, ack-after-outcome)** — `waker.go`
  creates `CreateOrUpdateConsumer` per registration on
  `realm.NotifyStreamName` filtered to `topic.NotifySubject(persona)`,
  AckExplicit, `AckWait = run_timeout + margin`, server MaxDeliver
  unlimited (the budget is waker policy — a server-side drop would be a
  silent hole, constitution V). `wake.go` acks only in the three outcome
  arms; every other path naks with a class-appropriate delay.
- **FR-003 (probe)** — per wake, `realm.Connect` as the agent
  (token lane: URL + sentinel + token; ephemeral lane: mint then
  connect), close immediately. Two non-admitted classes per research D4.
- **FR-004 (templates are configuration)** — `registration.go` loads and
  validates; `harness.go` interprets; nothing imports a harness name.
  SC-004's guard: the integration test runs both grammars through one
  binary.
- **FR-005 (reply obligation, no duplicates)** — `correlate.go`: the
  before/after set difference (measured D-fix from the research), the
  UUIDv5 wake op-id, and the redelivery pre-check (materialise, look for
  the wake op-id — D7). The reply posts through the **agent's own
  client** via `PostTurnIdempotent`; core's authorship mechanics
  (author == client persona, not a parameter) make cross-authorship
  impossible rather than merely forbidden.
- **FR-006 (failure in the waker's voice)** — a second, long-lived
  client bound to the waker's persona posts the failure turn with
  `PostTurnMentioning(body, [agent, asker])`, same idempotent wake
  op-id (one outcome slot per wake, whichever kind fills it).
- **FR-007 (credential lanes)** — `registration.go` declares the lane;
  the ephemeral lane consumes an `EphemeralMinter` narrow interface
  (satisfied by `soulstream-identity/client` in `cmd/`, faked in tests)
  — the same injection pattern as `runner.Minter`.
- **FR-008 (run hygiene)** — `harness.go` mirrors the measured rig:
  fresh dir under the scratch root, `SOULSTREAM_*`-scrubbed child env,
  stdin closed, `Setpgid` + group SIGKILL on timeout, outcome decided by
  the event stream never the exit code.
- **FR-009/FR-011/FR-012** — narration is parsed but only logged (D8);
  hermetic gate via `harness-mock`; the waker publishes outcome ops and
  nothing else.
- **Outcome-op publication after cancellation** — the discharge path
  uses `context.WithoutCancel` from the serve context, mirroring
  `runner.Running.base`: a shutting-down waker still finishes the wake
  it owes.

## Complexity Tracking

| Item | Why it exists | Simpler alternative rejected because |
|---|---|---|
| Core bump v0.8.0 → v0.8.3 (cross-repo prerequisite) | The token lane (`realm.Config` URL/CredsFile/Token), `PostTurnMentioning`, and the new `PostTurnIdempotent` (research D7) live there | Vendoring or Layer-0 re-implementation would duplicate record construction, signing, and mention-notify — the library exists to own exactly that |
| `PostTurnIdempotent` added to core (additive, v0.8.3) | FR-005's idempotency across redeliveries needs a caller-supplied op id; core's internal `presetID` machinery already does this for rollup | A waker-side marker header can't be set through the public API, and raw-wire publishing re-implements Layer 1 |
| `soulstream-identity/client` dependency (new edge) | FR-007's ephemeral lane is the identity plane's op by design (D28 was made for this repo's fleet) | Re-implementing sealed request/reply against the identity service is an audit surface this repo must not grow |
| Two live realm clients per waker (+ one per admitted wake) | Core stamps authorship from the client's persona — the agent's reply and the waker's testimony are different authors by mechanics | A single client with switchable authorship is exactly the attribution-laundering the design forbids |
| `log/slog` enters the repo (research D8) | Refused wakes are deliberately op-less; without a log they are invisible, failing constitution V's spirit | `fmt` to stderr has no levels or fields for a standing daemon; silence hides refusals |

# Implementation Plan: The standing dispatcher — agents as infrastructure

**Branch**: `011-dispatcher` | **Date**: 2026-08-28 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/011-dispatcher/spec.md`; soul-hq design [`0007-agents-as-infrastructure.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0007-agents-as-infrastructure.md) (graduated, episode 0141), composing [`0003-fleet.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0003-fleet.md) (built, M3.1), [`0004-wrap.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0004-wrap.md) §9 (the promised serve arm), [`0005-agent-declaration.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0005-agent-declaration.md) (built, specs/009) and [`0006-loop-safety.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0006-loop-safety.md) (built, specs/008).

## Summary

One new package composing two shipped ones. The dispatcher watches the
placement topic **live** (a subscription on its ops subject, with a
materialise poll as catch-up), races open agent placements through the
ordinary claim path it owns (the resolved [O]: option (b) — `fleet` gets
no launch hook), resumes from the log what it already owns without
publishing anything, and serves each won placement by running the
specs/009 wake engine for the declared persona on a client the caller's
`ConnectAgent` hook supplies. Liveness is `fleet`'s unchanged
three-step: this node answers probes for what it serves, and a
`Runner`-less `fleet.Node` sweeps peers on a cadence. The stop ceremony
is explicit — `Drain` cancels and waits so in-flight failures self-report;
crash semantics are the process dying. The 0006 budget is inherited, not
re-implemented: the dispatcher adds no admission point.

Held by the design at the operator's direction and therefore **not
built**: the `inference` block and provider-secret custody (0007 §3–§4).

## Technical Context

**Language/Version**: Go 1.26 (repo standard)
**Primary Dependencies**: existing only — this repo's `fleet`,
`declaration`, `wrap`; `soulstream-core` at the go.work sibling
(`realm`, `topic`); `nats.go` v1.52.0. **No new dependencies**, no
go.mod change, no core wire change, no soulstream-identity dependency
(unbroken since the daemon cut, design 0004 §9)
**Storage**: none — the record is the position (constitution
soulstream-workloads I). The only in-memory state is the set of
placements currently being served (the probe answer) and a bounded
node-local race backoff, both transient and both delaying decisions
rather than making them (design 0003 §1)
**Testing**: unit tests on the pure halves (config validation, the
servable predicate, backoff arithmetic — no NATS); embedded-JetStream
integration tests on the `integration/` rig shapes for design 0007 §8's
bars 1–3, with scripted invokers standing in for harnesses exactly as
`budget_test.go` does
**Target Platform**: wherever a node runs today (darwin/linux)
**Performance Goals**: a wake reaches a served engine on the live
subscription, not the poll; the poll is catch-up only. Reclaim is bounded
by node configuration (test scale ~1s, matching the spike's measured
~1.05s failover)
**Constraints**: `fleet`'s behavior byte-for-byte unchanged (its whole
diff is one exported identifier); no new op types; no new realm
vocabulary; no coordinator; refusals stay the engine's and stay op-less
**Scale/Scope**: one package added, one subcommand added, one identifier
exported in `fleet`; one integration file added; no existing test
modified

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **S1 NATS-Native First — PASS.** Everything is ordinary ops
  (`work.claim` / `work.abandon`) plus core-NATS transients that already
  exist (`fleet`'s probe subject). The watch is a plain subscription on
  the ops subject; there is no new stream, no new consumer, no durable
  position.
- **S2 Smallest Viable — PASS.** The dispatcher composes two shipped
  mechanisms and invents nothing. Option (b) was chosen precisely
  because it leaves `fleet` alone; the one export exists so the
  placement wire format keeps a single definition.
- **S3 Docs First — PASS.** Design 0007 is the normative home; this
  spec records its two resolved [O]s (the serve seam, the credential
  hook) and its one HELD block; the package doc carries the loop.
- **S4 Research Gates — PASS.** Episode 0141 measured all five bars;
  this feature turns bars 1–3 into standing tests and leaves 4–5 to the
  held inference work.
- **S5 All-Green — PASS.** `make check` before every commit; hermetic
  suites, zero skips; the new integration cases run `-race`.
- **soulstream-workloads I (Substrate Boundary) — PASS.** No durable
  truth in the dispatcher: resume reads the log, dedupe is the engine's
  outcome existence, and every position is an op. The owned set and the
  backoff die with the process and cost only a delay.
- **soulstream-workloads II (One Identity) — PASS.** A served agent is
  a persona with its own scoped credential, obtained through the hook;
  the dispatcher holds no elevated standing and no credential of its
  own beyond its node identity. Nothing branches on human vs machine.
- **soulstream-workloads III (Contracts Orthogonal to Backends) —
  PASS.** The declaration still names nothing about where or how it is
  served; engine-serve vs backend-launch is decided by the declaration's
  *role and wake set* (design 0007 §2's rule), never by a backend hint,
  and all dispatcher configuration is node-side.
- **soulstream-workloads V (Observable/Attributable) — PASS.** Claims,
  abandons and outcomes are attributed ops; the failed-serve abandon is
  visible; the probe/sweep transients stay off the stream by design and
  are asserted absent. The named limitation carried forward unchanged:
  design 0003 §3's zombie cap is still open.

Post-design re-check: unchanged — PASS on all articles.

## Project Structure

### Documentation (this feature)

```text
specs/011-dispatcher/
├── spec.md    # fixed decisions: serve seam (b), inference HELD,
│              # credential hook, drain-vs-crash
├── plan.md    # This file
└── tasks.md   # Phase 2
```

### Source Code (repository root)

```text
fleet/
└── fleet.go             # placementOf → DeclarationOf (exported, unchanged
                         #   behavior) — the placement wire format keeps one
                         #   definition

dispatcher/              # NEW
├── dispatcher.go        # the standing loop: watch (live + poll), scan
│                        #   (resume + race), claim path, serve, probe
│                        #   answers, sweep cadence, Drain
└── dispatcher_test.go   # pure-half units: startup refusals, the servable
                         #   predicate, defaults, backoff, Drain idempotence

cmd/soulstream-workloads/
└── main.go              # + `dispatcher serve`: node-side env config, the
                         #   per-persona creds-directory ConnectAgent lane,
                         #   drain on SIGINT/SIGTERM

integration/
└── dispatcher_test.go   # NEW: design 0007 §8 bars 1–3 as standing tests
```

**Structure Decision**: the dispatcher is its own package rather than a
`fleet` sub-mode, because it is a *composition* — it depends on `fleet`,
`wrap` and `declaration` at once, while `fleet` deliberately depends on
none of `wrap`. Putting it in `fleet` would drag the wake engine into
the placement plane and make a runner-only node link the harness
machinery. The dependency arrow stays one-way: `dispatcher → {fleet,
wrap, declaration}`, and nothing imports `dispatcher` but `cmd`.

## Complexity Tracking

No constitution violations to justify. Two judgment calls are recorded
in the spec rather than hidden here: the export of `fleet`'s
placement-body reader (one identifier, so the wire format is not
duplicated), and the node-local race backoff (transient, delays only —
without it a permanently-unservable declaration becomes a claim/abandon
spin *on the record*, which is worse than a little in-memory state).

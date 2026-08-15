# Implementation Plan: Wrap — run your agent where you are

**Branch**: `006-wrap` | **Date**: 2026-08-15 | **Spec**: [spec.md](spec.md)

## Summary

Rename the engine (`waker/` → `wrap/`), cut the daemon, and rebuild the
runtime half around position-is-the-record: `cmd/soulstream-wrap` connects
once as the agent, catches up from `topic.FetchInbox` against the
deterministic outcome id, then answers live arrivals from a raw
subscription on its own notify subject. Pure parts (template, harness
execution, correlation) carry over nearly untouched; the consumer, dialer,
and second-persona machinery are deleted.

## Technical Context

**Dependencies**: core stays at published v0.8.3 (`PostTurnIdempotent`,
`FetchInbox`, token-lane `realm.Config`); `soulstream-identity` and its
minting leave with the daemon (`go mod tidy` drops them); `google/uuid`
stays (wake ids). **Storage**: none. **Testing**: hermetic as specs/005 —
`natstest.StartJetStream` for protocol proofs, `StartOperator` for the
refusal, scripted harness via `cmd/harness-mock` (kept). **Platform**:
the person's own machine, one process.

## Constitution Check

- **workloads I (substrate)**: PASS — strengthened: even consumer state is
  gone; the wrapper's only durable artifacts are ops.
- **workloads II (one identity, no privileged tier)**: PASS — strengthened:
  the wrapper holds exactly the agent's credential; the daemon's
  support-layer standing is deleted, not just unused.
- **workloads III (contracts orthogonal)**: PASS by abstention (backend
  seam untouched); templates keep playing the analog on the harness axis.
- **workloads IV (research gates)**: PASS — episodes 0082/0083 are the
  measured basis; the reshape implements design 0004 as amended, and the
  cut records its reversal condition there.
- **workloads V (observable)**: PASS — outcomes are ops; refusals and
  skips are slog lines (the D8 precedent, kept).
- **S2 (smallest viable)**: PASS — this is a net deletion: no consumers,
  no dialer lanes, no minter interface, no second persona; presets exist
  because the two named harnesses are the concrete occupants.
- **S5**: hermetic default gate; `test-wake` target renamed `test-wrap`.

## Project Structure

```text
wrap/                    # renamed from waker/ (git mv; history preserved)
├── wrap.go              # package doc; Wrapper{Config, Client, Invoke, Log};
│                        #   Run(ctx): catch-up → live subscribe → sequential wakes;
│                        #   catch-up re-runs on NATS reconnect
├── wake.go              # per-wake protocol: self/type guards, outcome-existence
│                        #   check, in-process retry budget, discharge (reply as
│                        #   the agent | correlated | self-report), WithoutCancel
├── config.go            # single-agent Config; Template (+ env block); presets
│                        #   (claude, codex) with MCP env derived from lane env;
│                        #   template-file loading (strict decode, terminal required)
├── harness.go           # carried over + template env application
├── correlate.go         # carried over unchanged
└── *_test.go            # reshaped call-sequence + table tests
cmd/soulstream-wrap/
└── main.go              # env contract (identity + lane, the same names the MCP
                         #   door reads) + flags: --harness | --template, --scratch,
                         #   --run-timeout, --retries, --inbox-limit; slog to stderr
cmd/soulstream-workloads/main.go   # `waker serve` arm removed; workload start only
integration/wrap_test.go # hermetic SC-001..004 (renamed/reshaped from waker_test)
integration/wrap_live_test.go      # build tag wrap_e2e: real `claude -p`
Makefile                 # test-wake → test-wrap
```

## Complexity Tracking

| Item | Why |
|---|---|
| Sequential wakes, no concurrency knob | A laptop is not a fleet; the daemon that needed parallelism is cut (S2) |
| Presets in code rather than shipped files | Two named occupants; a file format would be speculative generality |

# Implementation Plan: Second backend — the same declarations under microVM isolation

**Branch**: `003-microsandbox-backend` | **Date**: 2026-07-28 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/003-microsandbox-backend/spec.md`

## Summary

Add a second implementation behind the existing `backend.Backend` seam that
runs each workload inside its own microsandbox microVM, and prove constitution
III by running the M1.1 agent and M1.2 tool declarations byte-identical under
it. The backend shells out to the `msb` CLI as its supervised child process
(research D1): credentials and scratch ride bind mounts with the same
`SOULREALM_*` env contract as native (D3), the guest reaches host NATS via
`host.microsandbox.internal` under the `host` network profile (D2), and
lifecycle maps SIGTERM/exit-code-for-exit-code onto the native semantics (D4).
Nothing above the seam changes: declaration, minter, runner, and reference
workloads are untouched except for backend selection in the CLI (node-side,
per FR-001).

## Technical Context

**Language/Version**: Go 1.24 (module `github.com/impire-io/soulrealm`)
**Primary Dependencies**: none added — the backend execs the `msb` CLI
(microsandbox 0.6.7, operator-installed); existing nats.go/jwt/nkeys +
soulstream client stay as-is. The Go SDK was evaluated and rejected (research D1).
**Storage**: none (constitution I — scratch dirs and sandboxes are reaped;
microsandbox's own image cache lives under `~/.microsandbox/`, operator-owned)
**Testing**: `go test ./...` hermetic (stub-CLI fakes, in-process NATS);
real-microVM e2e behind build tag `msb_e2e` via `make test-msb` (research D6)
**Target Platform**: macOS Apple Silicon + Linux hosts; guest is always
linux/<host-arch> (research D5)
**Project Type**: single Go module — runtime library + `cmd/soulrealm` CLI
**Performance Goals**: sandbox launch-to-ready seconds-scale; e2e discovery
retry window ≥ 15 s to absorb cold image pulls (research, measured)
**Constraints**: `make test` must pass with no msb installed; declarations
byte-identical across backends (SC-001/002); zero sandboxes/scratch/creds
left after any end-of-life (SC-004)
**Scale/Scope**: single node, two reference workloads, one new backend
package + CLI selection + e2e suite

## Constitution Check

*GATE: passed pre-research; re-checked post-design — no violations.*

- **I — Substrate boundary**: PASS. The backend stores nothing durable;
  sandbox records are removed on reap (`msb rm`), scratch dirs deleted, and
  everything worth keeping is already ops on the topic. The msb image cache is
  node-local operator state (like the Go toolchain), not a store of record.
- **II — One identity, no privileged tier**: PASS. Same minter, same
  per-workload scoped credential, delivered by creds file into the sandbox.
  The sandbox's network policy is *tighter* than native (deny-by-default,
  `host` group only) and grants no extra identity.
- **III — Contracts orthogonal to backends**: PASS — this feature *is* the
  proof. Declaration untouched (strict parsing already rejects backend
  fields); backend chosen node-side via CLI env (`SOULREALM_BACKEND`); the
  seam's meeting points (credential injection, lifecycle signals, scratch)
  documented in contracts/backend-seam.md.
- **IV — Research gates**: PASS. Phase 1 is unblocked (Phase 0 closed); this
  plan's research (research.md) resolved every unknown with measured evidence
  before build.
- **V — Observable, attributable execution**: PASS. The runner publishes the
  identical op mapping; the backend emits no ops and owns no control channel.
  Named limitation recorded (research D2): remote (non-loopback) NATS needs
  the `public` profile — a Fleet-era concern.
- **VI — All-green gate**: PASS by design. Hermetic default suite (stub CLI,
  in-process NATS, no skips); real-microVM proof in `make test-msb`; the M1.3
  exit gate is `make check && make test-msb` green on the operator's machine.

## Project Structure

### Documentation (this feature)

```text
specs/003-microsandbox-backend/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0: measured microsandbox findings (D1–D6)
├── data-model.md        # Phase 1: entities and state transitions
├── quickstart.md        # Phase 1: run the M1.3 slice by hand
├── contracts/
│   ├── backend-seam.md  # The seam contract both backends satisfy
│   └── workload-env.md  # The env/creds contract a workload sees (unchanged)
├── checklists/requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
backend/
├── backend.go           # UNCHANGED — the seam (Backend, LaunchSpec, Handle)
├── native/              # UNCHANGED — reference backend
└── msb/
    ├── msb.go           # Backend: msb-run arg construction, mounts, env,
    │                    #   net policy, URL rewrite; handle: Wait/Stop + reap
    └── msb_test.go      # hermetic unit tests against a stub msb executable

cmd/soulrealm/main.go    # backend selection: SOULREALM_BACKEND=native|msb
                         #   (+ SOULREALM_MSB_IMAGE override), fails loud on
                         #   unknown values; declaration handling untouched

integration/
├── msb_e2e_test.go      # build tag msb_e2e: SC-001 agent scenario, SC-002
│                        #   tool scenario, SC-003 isolation probe, SC-004
│                        #   crash→abandon + zero-leftovers sweep
└── helpers_test.go      # + buildCmdLinux (GOOS=linux CGO_ENABLED=0 build)

Makefile                 # + test-msb target (go test -tags msb_e2e ./integration/)
```

**Structure Decision**: mirror `backend/native/` with `backend/msb/` — the
package layout is itself the constitution-III statement: two siblings behind
one seam, runner and declaration untouched. E2E proof lives with the existing
integration package, gated by build tag so the default suite stays hermetic.

## Design outline (how the pieces satisfy the spec)

1. **`backend/msb.Backend`** holds node-side config only: `Image` (default
   `alpine`), `MsbPath` (default `msb`, overridable — this is also the unit-test
   seam), `HostAlias` (default `host.microsandbox.internal`). `Start`:
   - writes `nats.creds` into the scratch dir (same bytes as native);
   - derives the sandbox name from the scratch dir's work-item id
     (`soulrealm-<id>`);
   - builds `msb run --no-tty --quiet --name <name> -v <scratch>:/scratch
     --copy-file <artifact>:/artifact/<bin> -w /scratch --net host
     -e KEY=VAL … <image> -- /artifact/<bin> <args…>` (amended from the
     planned `:ro` artifact mount — research D3's honest record), with both
     node-side paths symlink-resolved (msb 0.6.7 cannot mount through
     symlinks — measured);
   - env block = native's contract with `SOULREALM_NATS_CREDS=/scratch/nats.creds`
     and loopback server URLs rewritten to the host alias (research D2);
   - starts the process; failure → same cleanup-and-error shape as native.
2. **`handle.Wait`** waits on the msb process (exit code = guest exit code,
   measured), then reaps: `msb rm --force <name>` + scratch removal. Signal
   exits map through the existing `statusOf` semantics so `runner.Outcome`
   yields the same terminal ops.
3. **`handle.Stop`** SIGTERMs the msb process with native's 5 s grace, then
   SIGKILL — measured to stop the sandbox immediately.
4. **CLI selection** (FR-001): `SOULREALM_BACKEND` env var, `native` default,
   `msb` opt-in, anything else an error before any op is published.
5. **E2E** (SC-001…SC-005): the existing launch/tool test bodies re-run with
   the msb backend and linux-built artifacts; plus the isolation probe (a
   workload that reads a host-only path — succeeds native, fails sandboxed)
   and a crash workload asserting `work.abandon` + `msb ls` empty afterwards.

## Complexity Tracking

No constitution violations; table not needed.

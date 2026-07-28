# Research: microsandbox as the second isolation backend

**Feature**: `003-microsandbox-backend` | **Date**: 2026-07-28
**Sources**: microsandbox v0.6.7 source (github.com/superradcompany/microsandbox,
HEAD `b0e8eaa`, 2026-07-27), docs.microsandbox.dev, and **local measurements on
the target machine** (macOS 26.5.1, Apple Silicon, msb 0.6.7 via
`brew install superradcompany/tap/microsandbox`). Evidence classes per the
working agreement: **[measured]** beats [mechanism-argument].

## D1 — Integration surface: `msb` CLI subprocess, not the Go SDK

**Decision**: The backend shells out to the `msb` CLI (`msb run …`) as its
child process, exactly as the native backend execs the artifact directly.

**Rationale**:
- The seam is structurally identical to native: one child process per
  workload, `Wait` = process wait, `Stop` = SIGTERM (escalate SIGKILL). All
  load-bearing behaviors were **[measured]** on the target machine:
  - guest command exit code propagates through `msb run` (guest `exit 7` →
    host exit 7);
  - SIGTERM to the `msb run` process ends it immediately (exit 143) and the
    sandbox transitions to `stopped`;
  - a bind-mounted, host-cross-compiled static Go binary executes in the
    guest with its exec bit intact;
  - stdout/stderr stream through to the parent.
- The official Go SDK (`github.com/superradcompany/microsandbox/sdk/go`)
  exists but is CGO/FFI-based: adopting it makes CGO mandatory for every
  `go build ./...` of this module, adds macOS hardened-runtime/entitlement
  concerns (`com.apple.security.hypervisor`), and pins us to a beta API that
  has already retracted a patch release for breaking wire changes
  (`sdk/go/go.mod` retracts v0.6.5). Rejected for M1.3; reconsider if the
  backend ever needs per-exec streaming control the CLI cannot give.

**Alternatives considered**: Go SDK (above); a JSON-RPC server API (does not
exist in v0.6.x — the old `msb server` daemon was removed); hand-rolling
against the host↔guest agentd protocol (internal, unstable, unnecessary).

## D2 — Guest→host NATS reachability

**Decision**: The workload reaches the host's NATS server at
`host.microsandbox.internal:<port>`; the backend rewrites loopback hosts
(`127.0.0.1`, `localhost`, `::1`) in the minted credential's server URLs to
that alias before injecting `SOULREALM_NATS_SERVERS`, and grants the sandbox
the `host` network profile (`--net host`).

**Rationale** **[measured]**: with a listener on host `127.0.0.1:8099`, a
guest `nc -z host.microsandbox.internal 8099` fails under the default policy
and succeeds with `--net host`. The default policy denies host/private/
loopback/metadata — deny-by-default is the posture we want; the backend
grants exactly the one destination group the workload needs. In-guest,
`/etc/hosts` maps the alias to a per-sandbox gateway IP and the host-side
proxy rewrites gateway-bound dials to host loopback
(`crates/network/lib/stack.rs:763-776`).

**Alternatives considered**: `--net-rule "allow@host:tcp:<port>"` (tighter,
per-port; viable refinement, not needed for M1.3 — the minted NATS credential
is the real authorization boundary, the network profile is defense in depth);
`public` profile (wrong tool — that is internet egress, not host loopback).

**Named limitation**: a NATS server that is *not* on the node's own loopback
(a remote realm server) would need the `public` profile instead of the
rewrite. Single-node M1.3 (episode 0003 scope) always has NATS reachable from
the host; multi-node arrives with Fleet.

## D3 — Credential, artifact, and scratch injection

**Decision**: Reuse the native backend's delivery shape: the workload's
scratch dir (host) bind-mounted read-write at `/scratch` (also the guest
workdir, `-w /scratch`), with `nats.creds` written into it before boot
exactly as native does; the artifact copied into the guest rootfs pre-boot
(`--copy-file <host>:/artifact/<bin>`) and exec'd from there. Env vars are
passed with `-e`: same `SOULREALM_*` contract as native, with
`SOULREALM_NATS_CREDS` pointing at the in-guest path
(`/scratch/nats.creds`) and `SOULREALM_NATS_SERVERS` rewritten per D2.

**Rationale**: the workload-facing contract (env names, creds file, cwd =
scratch) stays byte-for-byte the one M1.1/M1.2 workloads already speak — the
reference workloads run unmodified. The pre-boot copy is a strictly better
posture than any mount for the artifact: the host copy is never exposed to
the VM at all (guest writes land in its own throwaway COW layer). Exec-bit
preservation through `--copy-file` was **[measured]**.

**Amended during implementation (honest record)**: the plan first said
"artifact directory mounted read-only". The initial `:ro` probe failed — but
the failure was misdiagnosed: **msb 0.6.7 cannot mount any source whose path
traverses a symlink** ("Not a directory", os error 20 — and macOS tempdirs
live behind `/var → /private/var`). On resolved paths `:ro` works correctly
**[measured]**. The backend therefore resolves symlinks
(`filepath.EvalSymlinks`) on the scratch dir and artifact before invoking
msb, and `--copy-file` was kept for the artifact on its own merits (no host
exposure), not because `:ro` is unavailable.

**Alternatives considered**: read-only artifact-dir mount (works on resolved
paths; exposes the host directory to the guest); copying the artifact into
scratch (mixes it into the workload-writable area); guest-agent file writes
(SDK-only surface).

## D4 — Sandbox naming, lifecycle mapping, and reaping

**Decision**: One sandbox per launch, named `soulrealm-<workitem-id>` (the
same id that names the scratch dir — grep-able from topic op to sandbox).
`Handle.Wait` = wait on the `msb` process, then `msb rm --force <name>` and
remove the scratch dir. `Handle.Stop` = SIGTERM to the `msb` process,
escalate SIGKILL after the same 5s grace native uses.

**Rationale** **[measured]**: a named sandbox shows `running` in `msb ls`
while up and `stopped` after its `msb run` process is SIGTERMed; the stopped
record persists until `msb rm`. Explicit named create + explicit remove makes
reaping deterministic and lets the e2e suite assert **zero remaining
sandboxes** (SC-004) instead of trusting auto-cleanup timing.

**Alternatives considered**: unnamed ephemeral sandboxes (auto-removed —
**[measured]** — but leave nothing to assert against and no name linking a
running VM back to its work item); `msb stop` by name from a second process
(redundant — killing the supervising `msb run` already stops the VM).

## D5 — Guest image and artifact architecture

**Decision**: Default guest image `alpine` (configurable on the backend
value, node-side only). Reference workloads cross-compile with
`GOOS=linux GOARCH=<host arch> CGO_ENABLED=0` — on this machine linux/arm64.

**Rationale** **[measured]**: guest `uname -m` on Apple Silicon is `aarch64`;
there is no cross-arch emulation (libkrunfw is native-arch), so the guest
GOARCH always equals the host's. A `CGO_ENABLED=0` static binary is
libc-independent, so the minimal musl image suffices. Image pull happens on
first boot and is cached under `~/.microsandbox/` (`if-missing` policy).

**Alternatives considered**: `debian:12` (bigger, no added value for static
binaries); baking artifacts into images (an artifact-registry concern, out of
scope until the object-store artifact milestone).

## D6 — Hermetic testing strategy (constitution VI)

**Decision**: `make test` stays hermetic: the msb backend's unit tests fake
the CLI by pointing the backend's command name at a stub executable (a tiny
shell script in the test's temp dir) that records its argv and mimics
run/stop/rm behavior. The real-microVM proof lives in
`integration/msb_e2e_test.go` behind build tag `msb_e2e`, driven by a
dedicated `make test-msb` target that the M1.3 gate runs on this machine.

**Rationale**: constitution VI requires the default suite to have no external
dependency; a microVM runtime is one (same status as an external NATS server,
which the suite replaces with in-process servers). The build tag keeps the
e2e code compiled-never-skipped semantics honest: `go test ./...` neither
runs nor skips it (no `t.Skip` anywhere), while `make test-msb` runs it for
real — mirroring how SC-003 needed an operator-mode harness. The gate
recorded in the roadmap for M1.3 is `make check && make test-msb`, all green
on the operator's machine (SC-005).

**Alternatives considered**: `t.Skip` when msb is absent (violates "none
skipped"); requiring msb for `make test` (violates hermeticity and breaks
every CI/Linux-x86 contributor).

## Measured environment (for reproducibility)

- macOS 26.5.1 (Darwin 25.5.0), Apple Silicon (arm64), Xcode CLT present.
- msb 0.6.7 (`brew install superradcompany/tap/microsandbox`); `msb doctor`
  green; `MSB_HOME=~/.microsandbox`.
- Alpine sandbox cold boot + one command: seconds, not minutes; warm boots
  ~2-4s. E2E timeouts sized accordingly (tool-discovery retry window ≥ 15s).
- Known upstream issue to watch: #1180 (proxied guest TCP CLOSE_WAIT leak,
  256-slot table). Irrelevant at our connection volume (one long-lived NATS
  connection per workload) but recorded.
- Upstream bug found during implementation **[measured]**: mounts fail with
  "Not a directory" when the host source path traverses a symlink (e.g.
  anything under macOS `/var/folders/...`). Worked around in the backend by
  resolving symlinks before handover; candidate upstream report.

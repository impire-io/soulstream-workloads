# kubernetes-backend — investigation journey (opened 2026-07-29)

## 2026-07-29 — environment survey

Local tooling: `kubectl` and Docker Desktop (server 29.2.1) present; no
cluster and no kind/k3d/minikube. Installed `kind` via Homebrew and created a
throwaway cluster `soulrealm-research` (context
`kind-soulrealm-research`) — this is the expected shape of the opt-in gate
target named in the bars. The maintainer supplies the non-loopback NATS
environment for Bar 4 (not yet wired; NATS CLI has several existing contexts
to choose from when the time comes).

## 2026-07-29 — spike A: supervision mapping (Bar 3 mechanism)

**Hypothesis:** a runner-supervised pod (`restartPolicy: Never`) can
reproduce the `backend.Handle` contract — `Wait()` observes a terminal exit
status, `Stop()` maps to deletion with a grace period — and leaves nothing
behind.

**Protocol:** throwaway client-go harness (session scratchpad,
`spike-a-supervision/`) against the kind cluster, fresh namespace, three
scenarios on `busybox:1.36`: self-exit 0, self-exit 3, and delete-with-
grace=5s while running `sleep 300`. A field-selector watch captures every
status update; cleanliness asserted by listing the namespace afterward.

**Results (single run, 2026-07-29) — all [measured]:**

| scenario | phase | exitCode | signal | reason | create→Running | Running→end |
|---|---|---|---|---|---|---|
| exit 0 | Succeeded | 0 | 0 | Completed | 3.452s (cold pull) | 1.175s |
| exit 3 | Failed | 3 | 0 | Error | 469ms (warm) | 1.53s |
| stop (grace 5s) | Failed | 137 | 0 | Error | 476ms | 5.571s |

Zero pods remained in the namespace after all three ends of life.

**Findings:**

- Exit codes map faithfully to `ContainerStateTerminated.ExitCode` [measured].
- **`Signal` is not populated** — SIGKILL after grace surfaced as
  exitCode 137 (128+9), signal 0 [measured]. `backend.ExitStatus.Signal`
  must be inferred from the 128+n convention, and a workload that literally
  exits 137 is indistinguishable from a SIGKILLed one — named-limitation
  candidate for the design [mechanism-argument].
- Stop-as-deletion works: Kubernetes sends TERM at delete (PID-1 `sh`
  without a handler ignored it), KILL follows after the grace period —
  Running→end 5.571s ≈ grace + kill [measured]. The seam difference vs the
  native backend: `Stop(ctx)` escalation is ctx-driven natively but
  gracePeriodSeconds-driven here, so the backend must translate the ctx
  deadline into a grace period at delete time [mechanism-argument].
- The termination state **was observable through the watch before the
  object disappeared**, so `Wait()` after `Stop()` can return a real status
  — but this was one run; a real backend must capture-on-every-update the
  way the spike does, not read status after deletion [measured, n=1].
- Startup latency ~0.5s warm / ~3.5s with image pull — slower than native,
  same order as microsandbox [measured].
- Dependency weight of the idiomatic client: client-go v0.36.3 pulls a
  68-module graph, 115-line go.sum, in the spike module [measured]. Whether
  the runner takes that dependency or supervises `kubectl` as a child
  process (the msb pattern) is an open implementation question — not a bar.

**Verdict for the Bar 3 mechanism:** no blocker found; supervision parity
looks reachable. Bar 3 itself still requires the full op-sequence parity run
(crash → `abandon`) with the real runner, which is the Bar 1 integration
spike's job.

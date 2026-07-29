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

## 2026-07-29 — spike B: artifact into the pod (Bar 2 mechanism)

**Hypothesis:** a *generic* image can fetch a host-built artifact at pod
start and exec it — no per-workload image build, no image reference in any
declaration.

**Protocol:** tiny Go workload cross-compiled twice (linux/arm64 and
darwin/arm64 — the kind node is aarch64, matching host GOARCH), served from
the host over HTTP (a stand-in for the object-store channel; pods reach the
host at Docker Desktop's `192.168.65.254` alias). Two `busybox:1.36` pods:
`wget && chmod +x && exec` of the linux build (positive) and the darwin
build (deliberate platform mismatch).

**Results (2026-07-29) — [measured]:**

- Positive: phase `Succeeded`, exit 0, workload printed
  `os=linux arch=arm64` from inside the pod. Generic-image fetch-and-exec
  works; no per-workload image was built.
- Negative: the kernel refuses the Mach-O binary, and busybox `sh` then
  **falls back to interpreting the binary as a shell script** — pages of
  garbage `not found` errors, exit 2, phase `Failed`. Platform mismatch
  fails *late and unreadably* in-pod.

**Findings:**

- The Bar 2 mechanism holds: artifact delivery needs a generic runner image
  plus a fetch channel, nothing more [measured].
- Artifact resolution must verify platform *before* launch (ELF magic /
  GOOS-GOARCH check node-side) so a mismatch is a clean pre-launch error
  instead of in-pod garbage [mechanism-argument from the measured negative].
- Precedent already in the repo: M1.3's e2e test keeps the **declared
  artifact path stable and provisions its content per run** (host build for
  native, guest build for msb, same `file://` path —
  `integration/msb_e2e_test.go`). The Kubernetes backend extends node-side
  provisioning across a network boundary; it does not need new declaration
  vocabulary [measured — it is in the repo].
- The kind node arch equals host GOARCH under Docker Desktop, so msb's
  `GOOS=linux GOARCH=<host> CGO_ENABLED=0` static-build convention serves
  the kind gate target unchanged. A real org cluster may have nodes of a
  different arch than the runner host — artifact resolution then becomes
  node-arch-aware (multi-platform addressing, the shape OCI manifests
  solve). Named design question for graduation, not a research blocker
  [judgment].
- Channel choice (HTTP stand-in vs soulstream object store over `nats://`)
  is deliberately unresolved here — it interacts with Bar 4's connectivity
  wiring and is the design-level decision graduation must make.

**Next:** Bar 1 integration spike — a prototype `backend/k8s` in the
scratchpad wired to the *real* runner, byte-identical M1.1/M1.2
declarations, against a NATS the pods can reach (a spike server bound to
`0.0.0.0` reached via the host alias, or the maintainer's environment once
its coordinates arrive). Bar 4 waits on the maintainer's NATS environment.

## 2026-07-29 — spike C: Bar 1 integration — PASS (and Bars 2/3 with it)

**Protocol:** a prototype Kubernetes backend (~290 lines, scratchpad
`spike-c-bar1/`, never merged) implementing `backend.Backend` behind the
unchanged seam, driven by the **real** runner/minter/declaration imported
from the repo via `replace` — the working tree stayed byte-clean
throughout (`git status` empty, checked after the runs). The harness
mirrors `integration/msb_e2e_test.go` scenario for scenario against the
kind cluster; the in-process JetStream server binds `0.0.0.0` so pods reach
it at the Docker Desktop host alias, while runner and minter keep loopback
URLs — the backend's loopback rewrite (msb parity) is exactly what gets
exercised. Prototype shape: pod per workload (`restartPolicy: Never`,
name = work-item id, msb's naming convention), creds as a Secret mounted
read-only, scratch as in-pod `emptyDir` workdir, generic `busybox:1.36`
wget-and-execs the artifact staged on a node-side HTTP server, watch-based
supervision capturing termination state on every update (spike A lesson),
128+n signal inference, ELF check pre-launch (spike B lesson), reap of
pod + secret + staged artifact at end of life.

**Results (single run each, 2026-07-29) — all [measured]:**

- `TestK8sLaunchAgentEndToEnd` PASS 3.41s — ONE declaration value,
  marshalled byte-for-byte identical across the native control run and the
  pod run (asserted in-test); two turns by `researcher`, two `work.done`.
- `TestK8sAgentCallsToolEndToEnd` PASS 2.05s — tool inside a pod,
  discovered by name, uppercase round trip, `Stop` → `work.done`.
- `TestK8sCrashAbandons` PASS 2.00s — exit-3 in the pod → `work.abandon`.
- Zero pods and zero secrets after every scenario (asserted per-test and
  re-checked with `kubectl` after the suite: namespace empty).
- Whole tri-scenario suite: 7.9s.

**Bar verdicts this spike supports:**

- **Bar 1 — PASS [measured].** Byte-identical declarations, runner/minter/
  declaration untouched, no backend named anywhere but node-side wiring.
  The `backend.Backend`/`Handle` interface needed **no amendment**.
- **Bar 2 — PASS [measured].** No image reference, no Kubernetes-specific
  field; the same `file://` declaration served native and pod runs under
  the stable-path/provisioned-content convention. Channel decided for the
  spike: node-side HTTP fetch — the cheapest faithful mechanism; whether
  the real channel is the soulstream object store over `nats://` is a
  design decision (it rides the same connectivity Bar 4 proves), recorded
  for graduation.
- **Bar 3 — PASS [measured].** All three ends of life produced the
  native/msb op sequences through the real runner; exit status mapped
  (with the 128+n inference); nothing left behind.

**Honest caveats:** the spike NATS is the *open* in-process server — the
mint+delivery path is exercised, scope **enforcement** is not (same caveat
the msb e2e carries); that is Bar 4's job on the maintainer's environment.
`ScratchDir` in `LaunchSpec` is host-shaped — under Kubernetes it only
donates the work-item id as the pod name while scratch lives in-pod
(seam note for the design, not an interface change). The host alias is
Docker-Desktop-specific node config; a real cluster needs a routable NATS
address, which the maintainer's environment provides. All measurements are
single-run.

**Standing:** Bars 1–3 measured PASS; Bar 4 open, blocked on the
maintainer's non-loopback NATS environment (needs an operator-mode/scoped
setup so enforcement is real). When Bar 4 runs, the topic is ready for
`/research-graduate kubernetes-backend --to design`.

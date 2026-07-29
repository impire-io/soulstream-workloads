# Episode 0008 — A third wall on rented ground: Kubernetes as a backend (2026-07-28 → 2026-07-29)

The maintainer proposed supporting Kubernetes — not because it isolates best,
but because it is what organizations already run. Constitution III had named
it from day one; the open question was what it would *force* on the seams.
The `kubernetes-backend` research topic pre-registered four bars before any
spike ran, and four spikes later **all four are PASS [measured]**, each on a
single run against a throwaway kind cluster, with the last against Synadia
NGS:

- **Contract invariance.** A ~290-line prototype backend behind the
  *unchanged* `backend.Backend`/`Handle` interface, driven by the **real**
  runner/minter/declaration (repo working tree byte-clean after the runs).
  One declaration value, marshalled byte-identical across the native control
  run and the pod run — agent turn + `open/claim/done`, tool discovery +
  round trip + stop → `done`, crash → `abandon`. The tri-scenario suite ran
  in 7.9s [measured].
- **Artifact without leakage.** A stock generic image fetched a host-built
  static binary at pod start and exec'd it — no per-workload image, no image
  reference anywhere. M1.3's stable-declared-path / provisioned-content
  convention extended across the network boundary unchanged. The deliberate
  negative was instructive: a platform-mismatched (Mach-O) artifact fails
  *unreadably* in-pod — busybox `sh` interprets the binary as a shell script
  [measured] — so artifact resolution verifies ELF node-side, pre-launch.
- **Supervision parity.** `restartPolicy: Never`; `Stop` is deletion-with-
  grace (TERM at delete, KILL after); the termination state is watch-
  observable *before* the object disappears, so `Wait()` after `Stop()`
  returns a real status — but only if the backend captures on every update
  [measured]. Exit codes map faithfully; Kubernetes **never populates the
  Signal field** — signal deaths arrive as exitCode 128+n and are inferred,
  so a literal `exit 137` is indistinguishable from SIGKILL (named
  limitation) [measured]. Zero pods and secrets after every end of life.
- **Scoped credential over a real network.** The minter signed with the
  maintainer's NGS account signing key (read from disk at runtime, never
  logged); the credential reached the pod as a Secret-mounted file; the
  workload connected to `tls://connect.ngs.global` through ordinary pod
  egress; the operator-mode server allowed the in-scope publish and denied
  the out-of-scope one — from inside the pod [measured]. A native control
  arm ran the identical probe first so credential faults could not
  masquerade as pod faults.

Nothing was refuted, but one expectation inverted: on the non-loopback-NATS
axis **Kubernetes is ahead of the microVM backend** — msb needs the
Fleet-era `public` net profile (episode 0007's named limitation), while a
pod reaches a public NATS with no special networking at all [measured]. Two
further findings became design requirements: a TLS realm needs a **CA trust
store in the generic image** (busybox has none — Go's cert pool comes up
empty; alpine ships `ca-certificates-bundle`) [measured], and the
adversarial framing held all the way through — pods are *weaker* isolation
than microVMs; the case for this backend is **adoption, not isolation
strength** [mechanism-argument].

What it opened: design
[`0002-kubernetes-backend.md`](../02-DESIGN/0002-kubernetes-backend.md) —
pod-per-workload behind the unchanged seam — carrying the open internals
honestly: the artifact channel (node-side HTTP vs soulstream object store
over `nats://`), node-arch-aware resolution for heterogeneous clusters, the
128+n inference, Secret-at-rest exposure in etcd, and client-go's 68-module
dependency graph vs supervising `kubectl` in the msb style. The scope guard
stands: Kubernetes as a **backend, not a scheduler** — cluster-side
placement is a Fleet-era question. Roadmap Phase 2 (M2.1) is unblocked.

Reversal condition: if building the real backend shows the contract can only
be satisfied by leaking the backend — a declaration change or a
backend-conditional in the runner becomes *required* — the design is wrong
per constitution III: redesign the seam or abandon the backend. If
Kubernetes restart/eviction semantics prove uncontrollable in production
clusters so that supervision cannot stay with the runner, "backend, not
scheduler" reverses and the question moves to the Fleet gate.

Trail: research topic `hq/01-RESEARCH/kubernetes-backend/` (removed at
graduation; full history in git) — opened `9429ee2`, spikes `eff0468`
`5444c7b` `a2c49b6` `5fc7a16`, verdict `f2f403c`; design
[`0002-kubernetes-backend.md`](../02-DESIGN/0002-kubernetes-backend.md);
spike code in the session scratchpad (throwaway, per how-we-work).

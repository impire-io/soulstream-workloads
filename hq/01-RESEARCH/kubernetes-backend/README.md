# Can Kubernetes serve as a third isolation backend without bending the seam?

**State:** active
**Started:** 2026-07-29

## Abstract

Kubernetes is what organizations already run; a pod is where soulrealm must be
able to put a workload if it wants to exist inside today's infrastructure. The
motivation is adoption, not isolation strength — a plain pod is a *weaker*
wall than the landed microsandbox microVM (shared kernel), and this topic
records that trade openly rather than letting the two get conflated.
Constitution III already names Kubernetes as an anticipated backend; what is
genuinely unknown is what it forces on the seams: artifact distribution (a
cluster cannot read a `file://` path on the runner's machine) and credential
wiring to a non-loopback NATS. A decisive answer either lands the third wall —
proving the seam against the most opinionated substrate yet — or shows
precisely where the seam bends, which is worth as much.

## The question

Can Kubernetes serve as a **pure backend** behind the constitution III seam —
runner-supervised pods, byte-identical declarations, no scheduling opinions —
and what does it force on the artifact and credential seams?

Scope guard: Kubernetes **as a backend only**. Kubernetes-as-scheduler
(letting the cluster place workloads across nodes) is a Fleet-era question and
explicitly out of scope; if the investigation starts needing node-placement
logic, that finding graduates into the Fleet gate, not this topic.

## Pre-registered bars

- **Bar 1 — Contract invariance.** The byte-identical M1.1/M1.2 declarations
  (agent turn + `work.open/claim/done`; tool discovery + request-reply round
  trip) run unchanged under a Kubernetes backend spike, asserted in-test the
  same way the msb proof did (journey 0007). Runner, minter, and declaration
  packages untouched — a new backend package plus node-side wiring is the only
  allowed surface. Fail if any declaration field or runner conditional must
  name the backend.
- **Bar 2 — Artifact reaches the pod without leakage.** A workload artifact
  executes inside a pod while the declaration carries no image reference and
  no Kubernetes-specific field. Protocol: a generic runner image fetches the
  artifact at pod start (soulstream object store via `nats://`, or an
  equivalent host-fed channel — the spike decides which and records why). Pass
  if the artifact vocabulary that serves the native backend also serves
  Kubernetes.
- **Bar 3 — Supervision parity, nothing left behind.** With `restartPolicy:
  Never` (supervision stays with the runner), the runner-published op
  sequences match native/msb for all three ends of life: normal exit, crash →
  `abandon`, and `Stop` → pod deleted. Exit status maps from container
  termination state to `backend.ExitStatus`. Zero pods and zero injected
  secrets remain after every end-of-life, asserted by listing the namespace in
  the spike.
- **Bar 4 — Scoped credential over a real network.** The minted persona-scoped
  credential reaches the pod (Secret / projected file), the workload connects
  to the maintainer-provided non-loopback NATS, and scope enforcement holds:
  the M1.1-style scope-violation probe is denied from inside the pod. Server
  provisioning is out of scope — the maintainer supplies the running NATS
  environment; this bar measures wiring and enforcement only.

Gate cost, named now: a real end-to-end run needs a cluster, so the expected
shape is an opt-in target against kind/k3d following the `make test-msb`
pattern — the default gate stays hermetic.

## Reversal condition

- If satisfying the contract **requires** a declaration change or a
  backend-conditional in the runner — i.e. the contract can only be satisfied
  by leaking the backend — then per constitution III the design is wrong:
  either the seam itself gets redesigned (a new topic) or Kubernetes is
  abandoned as a backend. Bar 1 failing after honest attempts is this
  evidence.
- If runner-supervised pods prove unworkable — Kubernetes restart/eviction
  semantics cannot be held off so that supervision stays with the runner (Bar
  3 unreachable) — then "Kubernetes strictly as a backend" reverses, and the
  question moves to the Fleet gate as "Kubernetes-as-scheduler or nothing".

## Verdict

<Empty until graduation. Filled by /research-graduate: PASS/FAIL per bar with
the honest findings, each load-bearing claim tagged [measured] /
[mechanism-argument] / [judgment].>

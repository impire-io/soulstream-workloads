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

**All four pre-registered bars: PASS.** Four spikes (A–D, 2026-07-29; full
protocols and raw readings in `JOURNEY.md`), run on a throwaway kind cluster
(`soulrealm-research`) with Bar 4 against Synadia NGS. Single runs each.

- **Bar 1 — Contract invariance: PASS [measured].** A ~290-line prototype
  backend behind the unchanged `backend.Backend`/`Handle` interface, driven
  by the real runner/minter/declaration (imported via `replace`, working
  tree byte-clean after the runs). ONE declaration value marshalled
  byte-identical across the native control run and the pod run; agent flow,
  tool flow, and crash flow all green. No interface amendment was needed.
- **Bar 2 — Artifact without leakage: PASS [measured].** A stock generic
  image (`busybox`/`alpine`) fetched a host-built static binary at pod start
  and exec'd it — no per-workload image, no image reference or
  Kubernetes-specific field anywhere. M1.3's stable-declared-path /
  provisioned-content convention extended unchanged. Channel used: node-side
  HTTP (cheapest faithful mechanism); object store over `nats://` is the
  design-level candidate [judgment]. Platform mismatch fails unreadably
  in-pod (kernel refuses Mach-O, `sh` interprets it as script) [measured] —
  so resolution verifies ELF node-side, pre-launch.
- **Bar 3 — Supervision parity, nothing left behind: PASS [measured].**
  `restartPolicy: Never`; normal exit → `work.done`, crash → `work.abandon`,
  `Stop` → delete-with-grace → `work.done`, driven by the real runner. Exit
  codes map faithfully; Kubernetes never populates the Signal field — a
  signal death arrives as exitCode 128+n and is inferred, so a literal
  `exit 137` is indistinguishable from SIGKILL (named limitation). Zero pods
  and secrets after every end of life.
- **Bar 4 — Scoped credential over a real network: PASS [measured].** The
  minter signed with the maintainer's NGS account signing key (read from
  disk, never logged); the credential reached the pod as a Secret-mounted
  file; the workload connected to `tls://connect.ngs.global` through
  ordinary pod egress; in-scope publish allowed, out-of-scope denied by the
  operator-mode server — from inside the pod. New finding: a TLS realm
  requires a CA trust store in the generic image (busybox has none; alpine
  ships one) [measured].

One expectation inverted rather than refuted: on the non-loopback-NATS axis
Kubernetes is *ahead* of the microVM backend (msb needs the Fleet-era
`public` net profile; a pod needs nothing special) [measured]. The
adversarial note held: pods are weaker isolation than microVMs — the case
for this backend is adoption, not isolation strength [mechanism-argument].

Carried to the design: artifact channel ([O]), node-arch-aware resolution
for heterogeneous clusters ([O]), the 128+n signal inference (named
limitation), Secret-at-rest exposure in etcd (named consideration), and
client-go's 68-module graph vs supervising `kubectl` msb-style ([O]).

# Episode 0009 — A third wall lands: the Kubernetes backend ships (2026-07-29)

M2.1 went from spec to green in one spec-kit pass
([`specs/004-kubernetes-backend/`](../../specs/004-kubernetes-backend/)),
built directly on episode 0008's measured ground. The landed shape:
`backend/k8s` runs one runner-supervised pod per workload behind the
unchanged seam — the credential as a read-only Secret that **never touches
host disk** (tighter than native/msb) [measured: asserted in the unit
suite], scratch as an in-pod emptyDir workdir, supervision as a watch
established *before* the pod exists that captures termination state on
every update, and the artifact as a **per-run OCI image**: layered onto the
CA-trusted alpine base, digest-pinned, pushed with go-containerregistry (no
builder daemon), run by the pod as its entrypoint with no fetch machinery
inside. Five e2e scenarios ran green on a real kind cluster with a local
OCI registry — the true assemble → push → kubelet-pull path — in ~26 s
[measured]: the byte-identical M1.1 agent against a native control arm,
the M1.2 tool round trip with stop → `work.done`, crash → `work.abandon`,
an out-of-band pod deletion still closing as `work.abandon` with **no
resurrected copy**, and the new `cmd/scope-probe` reference workload denied
out-of-scope from inside a pod by an operator-mode server. `make check`
stays hermetic (fake clientset, in-process registry and NATS) and
`make test-msb` stays green — M1.3 survived the one refactor below the
seam, the extraction of the shared `backend/natsurl` rewrite helper
[measured].

Two reversals/decisions are on record from the planning pass. **The
artifact channel reversed:** the plan's first draft chose a node-staged
HTTP listener; the maintainer rejected it — a bespoke interface where a
standard one exists — and decided an **OCI-registry interface**, an open
amendment that chose *neither* of design 0002's graduation-time candidates
(node HTTP, object store over `nats://`). The mechanism argument held up in
practice: content addressing, auth, and caching came free, and the in-pod
fetch script disappeared entirely [mechanism-argument, then measured by the
green path]. **The client internal was decided after teach-back:**
client-go wrapped entirely inside `backend/k8s` — the typed watch is the
load-bearing operation and the fake clientset became the hermetic seam;
supervising `kubectl` (unstable output contract, unmeasured) and writing
our own API client (a hand-written NATS-client equivalent) were argued and
rejected [judgment, recorded with the argument].

Two smaller lessons: the `/speckit-analyze` remediation earned its keep —
its finding that `natstest` needed bind-address options on *both* the
JetStream and operator servers (not just the operator one) would otherwise
have surfaced mid-MVP [measured: T009 depended on it]; and the linter
caught that tag-shared e2e helpers must live behind a combined build tag
(`msb_e2e || k8s_e2e`) or the hermetic build carries dead code.

What it opened: Phase 2 is complete — soulrealm now runs the same
declarations on the infrastructure organizations already operate. The named
follow-ons stand unchanged: node-arch-aware artifact resolution for
heterogeneous clusters (design 0002 §3 `[O]`), the `nats://`
declared-addressing question at the artifact-registry milestone, and the
Fleet/sandboxes/tool-ecosystem horizons behind their research gates.

Reversal condition: carried forward from episode 0008, now hardened by a
landed implementation — if production clusters show the contract can only
be satisfied by leaking the backend (a declaration change or a
backend-conditional in the runner becoming *required*), constitution III
says the design is wrong: redesign the seam or abandon the backend. If
runner-supervised pods prove uncontrollable on real clusters (restart or
eviction semantics that cannot be held off), "backend, not scheduler"
reverses and the question moves to the Fleet gate.

Trail: [`specs/004-kubernetes-backend/`](../../specs/004-kubernetes-backend/)
(spec, plan with the recorded HTTP→OCI reversal, research D1–D7, tasks);
design [`0002-kubernetes-backend.md`](../02-DESIGN/0002-kubernetes-backend.md)
(amended: OCI channel, client-go, gate shape) and
[`0001-soulrealm-runtime.md`](../02-DESIGN/0001-soulrealm-runtime.md) §6/§8;
code `backend/k8s/`, `backend/natsurl/`, `cmd/scope-probe/`,
`integration/k8s_e2e_test.go`, `scripts/kind-registry.sh`; branch
`004-kubernetes-backend` commits `d74cb5a` (spec) `7efb721` (plan)
`cbc6579` (OCI reversal) `38d5086` (tasks) `5e0d205` (analyze) `db4a9dd`
(core) `3f0e372` (e2e) + the landing commit carrying this episode.

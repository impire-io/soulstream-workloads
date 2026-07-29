# 0002 — The Kubernetes backend

**Status of this document:** graduated from the `kubernetes-backend` research
topic (episode 0008). All four pre-registered bars were **measured PASS**
via spikes on a kind cluster, the last against Synadia NGS (operator-mode,
enforcement real). Tags mark what those spikes validated **[V]**, what is
designed on top of them **[D]**, and the internals still open **[O]**.
Written functional-level so `/speckit-specify` can turn it into a feature
spec without guessing.

Maturity tags per [`README.md`](README.md). Seam vocabulary per
[`0001-soulrealm-runtime.md`](0001-soulrealm-runtime.md) §6 and the frozen
contract in
[`specs/003-microsandbox-backend/contracts/backend-seam.md`](../../specs/003-microsandbox-backend/contracts/backend-seam.md).

---

## 1. The capability

A third isolation backend runs any declared workload as **one Kubernetes pod
per workload**, runner-supervised, behind the unchanged backend seam
(constitution III). The same declarations that run under the native and
microsandbox backends run unchanged as pods — proven byte-identical in the
research spikes `[V]`.

The case for this backend is **adoption, not isolation strength**: a plain
pod is *weaker* isolation than a microsandbox microVM (shared kernel). That
trade is recorded openly (episode 0008); a node operator choosing this
backend chooses reach into existing infrastructure, not a stronger wall.

**Scope guard — backend, not scheduler.** The runner creates, watches, and
deletes pods on one cluster it is configured for; it makes no placement
decisions and uses none of Kubernetes' scheduling vocabulary beyond pod
creation. Location-transparent scheduling across nodes or clusters is the
Fleet horizon's question, explicitly not this backend's.

## 2. The seam contract

- The backend implements soulrealm's `backend.Backend`/`Handle` interface
  **unchanged** — the research needed no interface amendment `[V]`.
- The workload sees the identical `SOULREALM_*` environment contract as
  under native/msb, values adapted to the pod `[V]`: creds file at an
  in-pod path, loopback NATS URLs rewritten to a node-side host alias,
  non-loopback URLs passed through untouched (both branches exercised
  `[V]`).
- The declaration MUST NOT name the backend, an image, or any Kubernetes
  concept; backend selection stays node-side (`SOULREALM_BACKEND`,
  M1.3 convention) `[D]`.
- Like every backend it publishes **no ops** and owns no control channel;
  lifecycle publication is the runner's job (constitutions I and V) `[D]`.
- `LaunchSpec.ScratchDir` is host-shaped; under this backend it only
  donates the work-item id as the pod name (msb's sandbox-naming
  convention, sanitized to RFC 1123). Scratch itself is an in-pod
  `emptyDir` mounted at the workload's working directory (native parity),
  reaped with the pod `[V]`.

## 3. Artifact delivery

- **Generic runner image + fetch at pod start** `[V]`: a stock minimal
  image fetches the artifact bytes and execs them. No per-workload image is
  ever built; nothing image-shaped appears in any declaration.
- The node honours M1.3's **stable declared path / per-run provisioned
  content** convention: the backend reads the artifact bytes at `Start`
  time and stages them for the pod `[V]`.
- **Pre-launch platform verification** `[D]` (mechanism measured `[V]`):
  the backend MUST refuse a non-ELF artifact node-side before creating the
  pod — a mismatched binary otherwise fails unreadably in-pod (busybox `sh`
  interprets Mach-O as a shell script; episode 0008).
- **Channel** `[O]`: the spike used a node-side HTTP server (cheapest
  faithful mechanism). The design-level candidate is the soulstream object
  store over a `nats://` artifact scheme — which the declaration validator
  already anticipates — so every backend fetches from the realm's own
  store. The spec pass decides; the interface (fetch-then-exec inside a
  generic image) is fixed either way.
- **Node architecture** `[O]`: the guest must match the cluster node's
  platform (linux/<node-arch>). On the dev machine the kind node matches
  host GOARCH `[V]`; heterogeneous real clusters need node-arch-aware
  artifact resolution (the shape OCI multi-arch manifests solve). Interface
  and default: single-arch clusters, resolution refuses a mismatch.

## 4. Credential delivery

- The minted persona-scoped credential is delivered as a **Kubernetes
  Secret mounted read-only** at the in-pod creds path `[V]` — created with
  the pod, deleted with the pod, TTL-bounded by the mint.
- **Named consideration:** a Secret rests in etcd for the workload's
  lifetime. The credential is short-lived and persona-scoped, which bounds
  the exposure; a cluster with etcd encryption at rest tightens it. This is
  recorded, not solved `[D]`.
- **TLS realms:** the generic image MUST carry a CA trust store — measured:
  busybox has none and Go's cert pool comes up empty; alpine ships
  `ca-certificates-bundle` and connected to NGS over TLS end-to-end `[V]`.
  The default image is therefore one with CA trust.

## 5. Supervision and exit mapping

- Pod created with `restartPolicy: Never` and a termination grace equal to
  the seam's stop grace; supervision stays with the runner `[V]`.
- The backend watches the pod and **captures the container termination
  state on every status update** — after deletion the object (and its
  state) is gone; capture-at-the-end loses the exit status of a stopped
  workload `[V]`.
- `Stop(ctx)` maps to pod deletion with a grace period: Kubernetes sends
  TERM at delete and KILL after the grace — the native SIGTERM→SIGKILL
  escalation expressed as `gracePeriodSeconds`, derived from ctx at delete
  time (an issued grace cannot be shortened afterwards — seam note) `[V]`.
- Exit mapping `[V]`: exit codes are faithful; Kubernetes **never
  populates the Signal field**, so a signal death (exitCode 128+n) is
  inferred. **Named limitation:** a workload that literally exits >128 is
  indistinguishable from a signalled one.
- End of life reaps **everything Start created** — pod, Secret, staged
  artifact — on every path (self-exit, crash, stop): zero leftovers,
  asserted per-scenario in the research `[V]`.

## 6. Node-side configuration surface

All of it node configuration; none of it may appear in a declaration.

- Cluster access: kubeconfig + context (or in-cluster config), target
  namespace `[D]`.
- Generic image (default: a minimal image with CA trust, §4) `[D]`.
- Artifact serving/channel configuration (§3) `[O]`.
- Host alias for loopback rewrite (environment-specific; e.g. the Docker
  Desktop host address under kind) `[V]`.
- **Client internal** `[O]`: the spike used client-go (dependency weight
  measured: 68-module graph). The alternative is supervising `kubectl` as a
  child process (the msb pattern, no heavy dependency). The spec pass
  decides; the seam is indifferent.

## 7. Quality gate shape

The default suite stays hermetic (no cluster, no external dependency —
constitution VI). Real end-to-end proof is an **opt-in target against a
local kind/k3d cluster** following the `make test-msb` pattern: without the
tag the tests do not exist; with it they must pass `[D]`.

## 8. Acceptance criteria

The spec-kit feature out of this doc must demonstrate, mirroring the
research bars on a real cluster:

1. The byte-identical M1.1/M1.2 declarations run as pods with the identical
   op mapping — agent turn + `work.open/claim/done`; tool discovery +
   round trip + stop → `work.done` — with a native control arm asserting
   the declaration byte-for-byte.
2. A crash inside the pod ends as `work.abandon`.
3. A scope-violation probe run from inside a pod against an operator-mode
   NATS is denied out-of-scope and allowed in-scope, with the credential
   delivered as a Secret.
4. Zero pods and zero Secrets remain after every end of life.
5. Runner, minter, and declaration packages are untouched; the backend
   lands as a new package behind the existing seam.

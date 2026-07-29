# 0002 — The Kubernetes backend

**Status of this document:** graduated from the `kubernetes-backend` research
topic (episode 0008); **built and landed as M2.1**
([`specs/004-kubernetes-backend/`](../../specs/004-kubernetes-backend/),
episode 0009). All four pre-registered research bars were **measured PASS**
via spikes, and the implementation's five e2e scenarios run green on a real
cluster (`make test-k8s`). The `[O]`s this document carried were decided in
the spec-kit pass: the **artifact channel** landed as a per-run OCI image
via the operator's registry — an **open amendment**, decided by the
maintainer during planning; neither of the two candidates listed at
graduation (node-side HTTP, object store over `nats://`) was chosen — and
the **client internal** landed as client-go wrapped inside `backend/k8s`.
Tags mark what is validated **[V]** and what remains open **[O]**.

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

- **Per-run OCI image via the operator's registry** `[V]` (landed M2.1; an
  open amendment to this document's graduation-time candidates — decided by
  the maintainer in the specs/004 plan, research D2): the backend layers
  the resolved artifact bytes onto the CA-trusted base image (binary at
  `/workload`, the entrypoint), pushes **digest-pinned** to an
  operator-configured OCI registry (pure-Go assembly, no builder daemon),
  and the pod runs that image as its single container. The kubelet pulls it
  like any image — authenticated, digest-verified, cached. No fetch
  machinery exists inside the pod; integrity is the image digest.
- The node honours M1.3's **stable declared path / per-run provisioned
  content** convention: declarations stay `file://` and byte-identical; the
  image reference exists only in the pod spec the node writes `[V]`.
- **Pre-launch platform verification** `[V]`: the backend refuses a
  non-ELF artifact node-side before any registry or cluster call — a
  mismatched binary otherwise fails unreadably in-pod (episode 0008).
- The registry holds per-run images as **transport cache under operator
  retention** — content derived from the declared artifact, not a store of
  record (constitution I). The soulstream object store over `nats://`
  remains the eventual *declared-addressing* question, deferred to the
  artifact-registry milestone.
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
- **TLS realms:** the base image MUST carry a CA trust store — measured:
  busybox has none and Go's cert pool comes up empty; alpine ships
  `ca-certificates-bundle` and connected to NGS over TLS end-to-end `[V]`.
  The default base (`alpine:3.22`) therefore carries trust, and every
  per-run artifact image inherits it.

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

All of it node configuration (`SOULREALM_K8S_*`); none of it may appear in
a declaration. As landed `[V]`:

- Cluster access: kubeconfig via client-go's standard loading rules +
  `SOULREALM_K8S_CONTEXT`; target namespace `SOULREALM_K8S_NAMESPACE`.
- `SOULREALM_K8S_REGISTRY` (required): the OCI repository prefix per-run
  artifact images are pushed to and pulled from.
- `SOULREALM_K8S_BASE_IMAGE` (default `alpine:3.22`): the CA-trusted base.
- `SOULREALM_K8S_HOST_ALIAS`: loopback-rewrite target
  (environment-specific; e.g. the Docker Desktop host address under kind).
  A loopback realm with no alias fails loud pre-launch.
- **Client internal** (decided in the specs/004 plan, research D1):
  client-go, wrapped entirely inside `backend/k8s` — the typed watch is the
  load-bearing operation, and the fake clientset is the hermetic test seam.
  Nothing above the seam sees a Kubernetes type.

## 7. Quality gate shape

The default suite stays hermetic (fake clientset, in-process registry and
NATS — constitution VI). Real end-to-end proof is `make test-k8s` against a
local kind cluster + local OCI registry (`scripts/kind-registry.sh up`),
build-tagged `k8s_e2e`: without the tag the tests do not exist; with it
they must pass `[V]`. The M2.1 gate is `make check && make test-k8s`.

## 8. Acceptance criteria

**All met at M2.1 landing** (episode 0009; `make test-k8s` green — five e2e
scenarios in ~26 s on kind, plus the hermetic unit suites). The spec-kit
feature out of this doc demonstrated, mirroring the research bars on a real
cluster:

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

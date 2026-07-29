# Research: Kubernetes as the third isolation backend

**Feature**: `004-kubernetes-backend` | **Date**: 2026-07-29
**Sources**: the graduated `kubernetes-backend` research topic — four spikes,
all four pre-registered bars measured PASS (journey episode
[0008](../../hq/04-JOURNEY/0008-kubernetes-backend.md), design
[0002](../../hq/02-DESIGN/0002-kubernetes-backend.md)); spike environment: kind
v0.30 cluster on Docker Desktop (macOS, Apple Silicon), client-go v0.36.3,
Synadia NGS for the enforcement bar. Evidence classes per the working
agreement: **[measured]** beats [mechanism-argument]. D1 and D2 are the two
design-0002 `[O]`s this plan decides; D3–D7 consolidate the spike-validated
mechanisms into build decisions.

## D1 — Cluster-client internal: client-go, not a supervised `kubectl`

**Decision**: The backend uses `k8s.io/client-go` (typed clientset), depending
on the `kubernetes.Interface` abstraction so the hermetic unit tests inject
`k8s.io/client-go/kubernetes/fake`.

**Rationale**:
- The supervision contract is *watch-shaped*: the termination state must be
  captured on every status update because it is unobservable after the pod
  object is deleted **[measured, spike A/C]**. client-go gives a typed watch
  and typed `ContainerStateTerminated`; every one of the four research bars
  ran through client-go end-to-end **[measured]** — choosing anything else
  means building on an unmeasured mechanism.
- The msb precedent (CLI as supervised child, research 003-D1) was driven by
  the SDK being CGO/FFI. client-go is pure Go — that disqualifier does not
  exist here. A `kubectl get -w -o json` child process would parse an output
  stream that is not a stable API, add `kubectl` as an operator-installed
  prerequisite on every node, and lose typed errors.
- The cost is dependency weight: a 68-module graph, 115-line go.sum
  **[measured]**. Accepted: pure Go, no CGO, and the fake clientset it brings
  is a *better* hermetic seam than 003's stub-CLI scripts (typed, in-process,
  watch-capable).

**Alternatives considered**: supervised `kubectl` (above — unstable watch
contract, extra prerequisite, unmeasured); a separate Go module to quarantine
the dependency (module complexity for no functional gain; reconsider only if
the graph ever bites downstream consumers).

## D2 — Artifact channel: node-staged ephemeral HTTP, digest-verified

**Decision**: The backend stages the artifact bytes per run and serves them
from a node-side ephemeral HTTP listener; the pod's generic image fetches the
per-run URL, **verifies a sha256 digest computed node-side**, marks it
executable, and execs it. The staged copy and listener are reaped with the
run. The soulstream object store over a `nats://` artifact scheme is
deferred to the artifact-registry milestone.

**Rationale**:
- The zero-diff criterion (SC-001/002) structurally forces this: the
  M1.1/M1.2 declarations say `file://`, and they must run byte-identical. Any
  channel is therefore *node-internal transport* for bytes the node already
  resolved — exactly M1.3's stable-declared-path / per-run-content
  convention, extended across a network hop. Fetch-then-exec by a generic
  image is the spike-proven mechanism **[measured, spikes B/C]**.
- An object-store channel today has a bootstrap problem: fetching from the
  object store requires a NATS client *inside* the pod before the workload
  starts — a soulrealm-owned fetch helper that is itself an artifact needing
  distribution. Plain HTTP needs only what stock minimal images already
  carry **[measured: busybox `wget` sufficed]**.
- The digest check is the integrity guard for crossing the pod network in
  clear: the backend computes sha256 at staging; the fetch step verifies
  before exec (stock `sha256sum -c`), so a corrupted or tampered transfer
  fails legibly pre-exec instead of executing wrong bytes
  [mechanism-argument].

**Alternatives considered**: soulstream object store over `nats://` (the
eventual shape per design 0001 §6 — blocked on new declaration vocabulary,
which the zero-diff bar forbids this milestone, and on the in-pod fetcher
bootstrap; revisit at the artifact-registry milestone); per-workload OCI
images (build+registry infrastructure, slow, and the image would leak into
observable pod spec); embedding the artifact in the Secret/ConfigMap (etcd
object cap ~1 MiB; the reference binaries alone are ~2.4 MB **[measured]**).

## D3 — Credential delivery: Secret mounted read-only; no creds bytes on host disk

**Decision**: The minted credential is written as a per-run Kubernetes Secret
(same name as the pod), mounted read-only at `/creds/nats.creds`;
`SOULREALM_NATS_CREDS=/creds/nats.creds`. The env contract is otherwise
byte-for-byte the native one, with loopback NATS URLs rewritten to a
node-side host alias — the rewrite logic is extracted from `backend/msb`
into a small shared helper so both backends use one implementation.

**Rationale**: Secret-mounted delivery + TLS to a real enforcing server was
the Bar 4 proof **[measured, spike D]**: in-scope allowed, out-of-scope
denied from inside the pod, Secret gone after reap. Unlike native/msb, this
backend never writes creds to host scratch at all — a strictly tighter
posture (the credential exists only in the minter's memory and the cluster's
secret store). Named consideration (design 0002 §4): the Secret rests in
etcd for the workload's lifetime, bounded by the mint TTL; etcd encryption
at rest is the operator's cluster concern.

**Alternatives considered**: env-var delivery (visible in pod spec /
`kubectl describe` — worse exposure than a volume); projected token /
external secret stores (heavier machinery, nothing to gain at this
milestone); writing creds into host scratch for parity (pointless — no
bind-mount exists to carry it).

## D4 — Supervision: `restartPolicy: Never`, watch, capture-on-every-update

**Decision**: One pod per workload, `restartPolicy: Never`,
`terminationGracePeriodSeconds` = the seam's 5 s stop grace. A supervision
goroutine watches the pod (field selector on its name) from `Start`,
capturing `ContainerStateTerminated` on **every** update; terminal phase or
a Deleted event ends the watch, then everything is reaped and the status is
delivered to `Wait`. `Stop(ctx)` deletes the pod with a grace period derived
from ctx at delete time (capped at the stop grace).

**Rationale** **[measured, spikes A/C]**: exit codes surface faithfully
(0 → `Succeeded`, 3 → `Failed`/code 3); deletion delivers TERM then KILL
after the grace (observed 137 ≈ grace + kill, 5.571 s); the termination
state was observable through the watch before the object vanished — and is
gone after. Capture-at-the-end is therefore a correctness bug, not a style
choice. An out-of-band deletion (cluster evicts the pod) surfaces as the
Deleted event → nonzero/unknown status → `work.abandon`: supervision
survives external interference by construction.

**Exit mapping**: `Code` faithful; Kubernetes never populates the Signal
field **[measured]**, so exitCode > 128 maps to `ExitStatus{Signal:
name(code−128)}` — same shape `runner.Outcome` already consumes. **Named
limitation** (design 0002 §5): a workload that literally exits > 128 is
indistinguishable from a signal death; the terminal op (abandon) is correct
either way. Deleted-before-any-termination-state maps to `Code: -1`
(uncoded failure), the seam's existing convention.

**Alternatives considered**: polling (races the deletion window the research
measured); informers (machinery for caching many objects; one pod needs one
watch); Jobs instead of bare pods (adds a controller with its own retry
opinions — exactly what FR-008 must hold off).

## D5 — Pod identity, naming, and the zero-leftovers sweep

**Decision**: Pod and Secret are both named `soulrealm-<workitem-id>`
(scratch-dir base, msb's naming convention) sanitized to RFC 1123 lowercase,
and labeled `app.kubernetes.io/managed-by: soulrealm`. The e2e
zero-leftovers sweep lists by that label and asserts empty; `Wait`'s reap
deletes pod + Secret + staged artifact and stops the serve listener.

**Rationale**: one grep-able id from topic op → pod → Secret → staged file
(the 003-D4 property, kept). The label makes "zero workload pods remain"
assertable without name heuristics. RFC 1123 sanitization is forced by the
API **[measured: uppercase/underscore ids are rejected]**.

**Alternatives considered**: generateName + label-only linkage (loses the
op↔pod grep); a dedicated namespace per run (heavyweight; namespace
lifecycle is the operator's concern, not per-workload machinery).

## D6 — Generic image: CA-trusted minimal default, node-side override

**Decision**: Default image `alpine:3.22`; node-side override
(`SOULREALM_K8S_IMAGE`). The image contract is: a POSIX shell, `wget`,
`sha256sum`, and a CA trust store.

**Rationale** **[measured, spike D]**: busybox carries no CA bundle — Go's
cert pool comes up empty and a TLS realm (NGS) is unreachable; alpine ships
`ca-certificates-bundle` and the Bar 4 probe connected over TLS end-to-end.
A TLS-capable realm transport is the expected production posture, so the
default must carry trust; image choice stays invisible to declarations
(constitution III).

**Alternatives considered**: busybox (fails TLS realms — measured);
distroless (no shell — the fetch-and-exec step needs one until an in-pod
fetcher exists, see D2); a soulrealm-built image (becomes an artifact-
distribution problem of its own; rejected for the same reason as D2's
fetcher).

## D7 — Hermetic testing: fake clientset default, `make test-k8s` for the real cluster

**Decision**: `make test` stays hermetic: unit tests drive the backend
against `kubernetes/fake` (typed fake clientset with watch support),
asserting pod/Secret specs, watch handling, grace derivation, exit mapping,
and reap idempotency — no cluster, no network. The real-cluster proof lives
in `integration/k8s_e2e_test.go` behind build tag `k8s_e2e`, driven by
`make test-k8s` against a local kind cluster (operator-provided, like `msb`
for M1.3). The M2.1 exit gate is `make check && make test-k8s` green on the
operator's machine.

**Rationale**: constitution VI — the default suite has no external
dependency and skips nothing; the build tag keeps compiled-never-skipped
semantics (003-D6's pattern, unchanged). The e2e suite re-runs the M1.1/M1.2
scenario bodies with the k8s backend plus the crash and scope scenarios —
mirroring the research bars 1–4.

**Alternatives considered**: `t.Skip` without a cluster (violates "none
skipped"); envtest/kind-in-CI (infrastructure this repo doesn't have; the
opt-in local target is the constitution's shape for it).

## Measured environment (for reproducibility)

- Spike cluster: kind (`soulrealm-research`) on Docker Desktop 29.2.1,
  macOS Apple Silicon; node `aarch64` = host GOARCH (per 003-D5, guest
  builds are `GOOS=linux GOARCH=<host> CGO_ENABLED=0`).
- Pods reach the host at Docker Desktop's `192.168.65.254` alias
  (environment-specific node config — the host-alias knob, not a constant);
  pods reach public NATS (NGS) through ordinary egress with no special
  networking.
- Timing: pod create→Running ~0.5 s warm, ~3.5 s cold pull; delete with
  grace 5 s → terminal in ~5.6 s; tri-scenario e2e suite 7.9 s; Bar 4
  against NGS 11.5 s. E2E readiness/discovery windows sized ≥ 60 s to
  absorb cold pulls.
- client-go v0.36.3 against server 1.34 (kind default): no skew issues
  observed across the four spikes.

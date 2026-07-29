# Implementation Plan: Third backend — the same declarations as Kubernetes pods

**Branch**: `004-kubernetes-backend` | **Date**: 2026-07-29 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/004-kubernetes-backend/spec.md`

## Summary

Add a third implementation behind the existing `backend.Backend` seam that
runs each workload as one runner-supervised Kubernetes pod, and prove
constitution III a third time by running the M1.1 agent and M1.2 tool
declarations byte-identical under it. Every load-bearing mechanism is
spike-validated (journey episode 0008): the pod carries the same
`SOULREALM_*` env contract with the credential as a read-only Secret mount
(research D3), the artifact ships as a **per-run OCI image** layered onto a
CA-trusted base and pushed digest-pinned to the operator's registry — the
pod runs it directly, entrypoint = artifact (D2/D6), a client-go watch
supervises to terminal state capturing termination on every update (D1/D4),
`Stop` maps to deletion-with-grace, and reap removes pod + Secret (D5).
Nothing above the seam changes: declaration, minter, runner, and reference
workloads are untouched except node-side backend selection in the CLI
(FR-001). The two design-0002 `[O]`s are decided in research.md: client-go
over a supervised `kubectl` (D1), and — a maintainer decision revising this
plan's first draft — an OCI-registry artifact interface over both the
draft's node-staged HTTP and the object store (D2).

## Technical Context

**Language/Version**: Go 1.26 (module `github.com/impire-io/soulrealm`)
**Primary Dependencies**: adds `k8s.io/client-go` + `k8s.io/api` +
`k8s.io/apimachinery` (v0.36.x; 68-module graph, measured — research D1)
and `github.com/google/go-containerregistry` (pure-Go OCI assembly + push —
research D2); existing nats.go/jwt/nkeys + soulstream client stay as-is
**Storage**: none (constitution I — pod and Secret are reaped; the Secret's
in-cluster rest during a run is transient and named, research D3; per-run
artifact images in the operator's registry are transport cache under
operator retention, research D2)
**Testing**: `go test ./...` hermetic (`kubernetes/fake` clientset,
in-process NATS, in-process registry fake for assembly tests); real-cluster
e2e behind build tag `k8s_e2e` via `make test-k8s` against a local kind
cluster plus a local OCI registry (research D7)
**Target Platform**: any Kubernetes cluster reachable via kubeconfig;
dev-machine proof on kind (Docker Desktop, Apple Silicon); pod artifacts are
`GOOS=linux GOARCH=<node-arch> CGO_ENABLED=0` builds (single-arch clusters —
spec assumption)
**Project Type**: single Go module — runtime library + `cmd/soulrealm` CLI
**Performance Goals**: pod launch-to-ready seconds-scale (~0.5 s warm,
~3.5 s cold pull — measured); e2e readiness/discovery windows ≥ 60 s
**Constraints**: `make test` must pass with no cluster present;
declarations byte-identical across backends (SC-001/002); zero pods,
Secrets, or staged artifacts left after any end-of-life (SC-003); cluster
restart machinery held off (`restartPolicy: Never`, FR-008)
**Scale/Scope**: single node, one configured cluster+namespace, two
reference workloads, one new backend package + a shared URL-rewrite helper
+ CLI selection + e2e suite

## Constitution Check

*GATE: passed pre-research; re-checked post-design — no violations.*

- **I — Substrate boundary**: PASS. The backend stores nothing durable: pod
  and Secret are deleted at reap, and everything worth keeping is already
  ops on the topic. The cluster's image cache and the operator's registry
  hold per-run artifact images as *transport cache derived from the
  declared artifact*, under operator retention — node-local operator state,
  not a store of record (the msb image-cache precedent, research D2).
- **II — One identity, no privileged tier**: PASS. Same minter, same
  per-workload scoped credential; delivered as a read-only Secret readable
  by that workload's pod alone, and — tighter than native/msb — never
  written to host disk (research D3). The pod env is built from scratch
  (only the `SOULREALM_*` contract); soulrealm's own env, including the
  realm signing key, cannot leak. Enforcement from inside a pod was the
  research's Bar 4 proof.
- **III — Contracts orthogonal to backends**: PASS — this feature is the
  third proof. Declaration untouched (strict parsing already rejects
  backend fields); backend chosen node-side (`SOULREALM_BACKEND=k8s`); the
  seam contract of specs/003 (contracts/backend-seam.md) is satisfied
  unchanged, with the k8s-specific mapping documented in
  contracts/k8s-mapping.md.
- **IV — Research gates**: PASS. The milestone's gate is met — the
  `kubernetes-backend` topic graduated with all four bars measured
  (episode 0008); this plan's research.md consolidates those findings and
  decides the two remaining `[O]`s on measured evidence.
- **V — Observable, attributable execution**: PASS. The runner publishes
  the identical op mapping; the backend emits no ops and owns no control
  channel. Named limitations recorded, not hidden: the 128+n exit-code
  ambiguity (research D4) and out-of-band pod deletion surfacing as
  `work.abandon`.
- **VI — All-green gate**: PASS by design. Hermetic default suite (fake
  clientset, in-process NATS, no skips); real-cluster proof in
  `make test-k8s`; the M2.1 exit gate is `make check && make test-k8s`
  green on the operator's machine.

## Project Structure

### Documentation (this feature)

```text
specs/004-kubernetes-backend/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0: the two [O] decisions + spike-validated mechanisms (D1–D7)
├── data-model.md        # Phase 1: entities and state transitions
├── quickstart.md        # Phase 1: run the M2.1 slice by hand
├── contracts/
│   └── k8s-mapping.md   # How this backend satisfies specs/003's frozen seam
│                        #   contract (backend-seam.md + workload-env.md)
├── checklists/requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
backend/
├── backend.go           # UNCHANGED — the seam (Backend, LaunchSpec, Handle)
├── native/              # UNCHANGED — reference backend
├── msb/                 # touched ONLY to adopt the shared URL-rewrite helper
│                        #   (behavior identical; its tests keep it honest)
├── natsurl/
│   └── natsurl.go       # extracted loopback→alias rewrite (was unexported in
│                        #   msb); shared by msb + k8s — one implementation
└── k8s/
    ├── k8s.go           # Backend (node-side config; Start: ELF check,
    │                    #   image assemble+push, Secret, pod spec,
    │                    #   supervision goroutine) and handle (Wait/Stop,
    │                    #   exit mapping, reap)
    ├── image.go         # per-run OCI assembly (artifact layer on BaseImage,
    │                    #   entrypoint /workload) + digest-pinned push
    │                    #   (go-containerregistry)
    ├── k8s_test.go      # hermetic unit tests against kubernetes/fake
    └── image_test.go    #   + an in-process registry for assembly/push

cmd/soulrealm/main.go    # backend selection: SOULREALM_BACKEND=native|msb|k8s
                         #   + SOULREALM_K8S_* node config (namespace, registry,
                         #   base image, host alias, kubeconfig context);
                         #   fails loud on unknown values; declarations untouched

integration/
├── k8s_e2e_test.go      # build tag k8s_e2e: SC-001 agent scenario, SC-002
│                        #   tool scenario, SC-003 crash→abandon + zero-
│                        #   leftovers sweep (by managed-by label), SC-004
│                        #   scope probe from inside a pod
└── helpers_test.go      # reused as-is (buildCmdLinux already exists)

Makefile                 # + test-k8s target (go test -tags k8s_e2e ./integration/;
                         #   expects kind + a local OCI registry, the documented
                         #   kind-with-registry pattern)
```

**Structure Decision**: mirror `backend/native/` and `backend/msb/` with
`backend/k8s/` — three siblings behind one seam is the constitution-III
statement in package layout. The one refactor below the seam extracts msb's
unexported URL-rewrite into `backend/natsurl` so the two rewriting backends
share a single implementation instead of a copy (reuse over duplication;
msb's behavior pinned by its existing tests). E2E proof joins the existing
integration package behind its own build tag, keeping the default suite
hermetic.

## Design outline (how the pieces satisfy the spec)

1. **`backend/k8s.Backend`** holds node-side config only: `Namespace`,
   `Registry` (push/pull target for per-run artifact images), `BaseImage`
   (default `alpine:3.22` — CA-trusted, research D6), `HostAlias` (loopback
   rewrite target), client as `kubernetes.Interface` (the fake-injection
   seam), kubeconfig/context resolution in the CLI. `Start`:
   - refuses a non-ELF artifact before any cluster or registry call
     (FR-009; research spike B's measured failure mode);
   - assembles the per-run OCI image — the artifact as a layer on
     `BaseImage`, placed at `/workload`, entrypoint `/workload` — and
     pushes it to `Registry` tagged with the work-item id, keeping the
     returned digest (research D2, go-containerregistry);
   - creates the Secret (`nats.creds` from the minted credential — never
     touching host disk, research D3), then the pod: single container
     running the digest-pinned per-run image, `command`
     `["/workload", spec.Args…]`, `restartPolicy: Never`, grace = stop
     grace, `emptyDir` at `/scratch` as workdir, Secret mounted read-only
     at `/creds`, env = native contract with creds path and rewritten
     servers (`backend/natsurl`);
   - names pod + Secret `soulrealm-<workitem-id>` (RFC 1123-sanitized) with
     the `app.kubernetes.io/managed-by: soulrealm` label (research D5);
   - starts the supervision goroutine (watch on the pod name, capture
     termination state on every update — research D4); failure anywhere →
     same cleanup-and-error shape as native (nothing left on the cluster).
2. **`handle.Wait`** blocks on the supervision result: terminal phase or
   Deleted event → map exit (`Code` faithful; Signal inferred 128+n; no
   state observed → `Code: -1`) → reap (delete pod grace-0 idempotently,
   delete Secret) → return status. Idempotent via `sync.Once`, the seam's
   convention.
3. **`handle.Stop`** deletes the pod with `gracePeriodSeconds` = min(stop
   grace, ctx remaining) — TERM at delete, KILL after grace (measured);
   the supervision watch picks up the terminal state before the object
   disappears, so the runner's `Stop → Wait → work.done` path reads a real
   status.
4. **CLI selection** (FR-001): `SOULREALM_BACKEND` gains `k8s`; unknown
   values still fail before any op is published. `SOULREALM_K8S_*` env
   config maps onto the Backend fields; kubeconfig context resolution via
   client-go's standard loading rules.
5. **E2E** (SC-001…SC-005): against kind plus a local OCI registry (the
   kind-with-registry pattern — the real assemble → push → kubelet-pull
   path): re-run the M1.1/M1.2 scenario bodies with the k8s backend and
   linux-built artifacts (native control arm asserting byte-identical
   declarations); crash workload asserting `work.abandon` + label-sweep
   empty; the scope probe from research Bar 4 run against the suite's
   operator-mode in-process NATS bound so pods can reach it (host alias),
   asserting in-scope allowed / out-of-scope denied from inside the pod;
   every scenario ends with the zero-leftovers sweep.

## Complexity Tracking

No constitution violations. Two deliberate dependency additions — client-go
(68-module graph) and go-containerregistry — are justified in research
D1/D2 against their alternatives; no table needed.

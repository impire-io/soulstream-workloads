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
(research D3), a generic CA-trusted image fetches the node-staged,
digest-verified artifact and execs it (D2/D6), a client-go watch supervises
to terminal state capturing termination on every update (D1/D4), `Stop`
maps to deletion-with-grace, and reap removes pod + Secret + staged
artifact (D5). Nothing above the seam changes: declaration, minter, runner,
and reference workloads are untouched except node-side backend selection in
the CLI (FR-001). The two design-0002 `[O]`s are decided in research.md:
client-go over a supervised `kubectl` (D1), node-staged HTTP over the
object store for this milestone (D2).

## Technical Context

**Language/Version**: Go 1.26 (module `github.com/impire-io/soulrealm`)
**Primary Dependencies**: adds `k8s.io/client-go` + `k8s.io/api` +
`k8s.io/apimachinery` (v0.36.x; 68-module graph, measured — research D1);
existing nats.go/jwt/nkeys + soulstream client stay as-is
**Storage**: none (constitution I — pod, Secret, and staged artifact are
reaped; the Secret's in-cluster rest during a run is transient and named,
research D3)
**Testing**: `go test ./...` hermetic (`kubernetes/fake` clientset,
in-process NATS); real-cluster e2e behind build tag `k8s_e2e` via
`make test-k8s` against a local kind cluster (research D7)
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
  and Secret are deleted at reap, the staged artifact and its listener are
  per-run and reaped, and everything worth keeping is already ops on the
  topic. The cluster's image cache is node-local operator state (the msb
  image-cache precedent).
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
    ├── k8s.go           # Backend (node-side config; Start: stage+digest,
    │                    #   Secret, pod spec, supervision goroutine) and
    │                    #   handle (Wait/Stop, exit mapping, reap)
    ├── serve.go         # per-run artifact staging + ephemeral HTTP listener
    └── k8s_test.go      # hermetic unit tests against kubernetes/fake
    └── serve_test.go    #   (specs, watch, grace, mapping, reap idempotency)

cmd/soulrealm/main.go    # backend selection: SOULREALM_BACKEND=native|msb|k8s
                         #   + SOULREALM_K8S_* node config (namespace, image,
                         #   host alias, kubeconfig context, serve address);
                         #   fails loud on unknown values; declarations untouched

integration/
├── k8s_e2e_test.go      # build tag k8s_e2e: SC-001 agent scenario, SC-002
│                        #   tool scenario, SC-003 crash→abandon + zero-
│                        #   leftovers sweep (by managed-by label), SC-004
│                        #   scope probe from inside a pod
└── helpers_test.go      # reused as-is (buildCmdLinux already exists)

Makefile                 # + test-k8s target (go test -tags k8s_e2e ./integration/)
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
   `Image` (default `alpine:3.22` — CA-trusted, research D6), `HostAlias`
   (loopback rewrite target), `ServeAddr` (artifact listener bind), client
   as `kubernetes.Interface` (the fake-injection seam), kubeconfig/context
   resolution in the CLI. `Start`:
   - refuses a non-ELF artifact before any cluster call (FR-009; research
     spike B's measured failure mode);
   - stages the artifact bytes under the pod name, computes sha256, serves
     both from the per-run listener (research D2);
   - creates the Secret (`nats.creds` from the minted credential — never
     touching host disk, research D3), then the pod: `restartPolicy: Never`,
     grace = stop grace, `emptyDir` at `/scratch` as workdir, Secret
     mounted read-only at `/creds`, generic-image command
     `fetch → sha256sum -c → chmod +x → exec "$@"` with `spec.Args`
     appended, env = native contract with creds path and rewritten servers
     (`backend/natsurl`);
   - names pod + Secret `soulrealm-<workitem-id>` (RFC 1123-sanitized) with
     the `app.kubernetes.io/managed-by: soulrealm` label (research D5);
   - starts the supervision goroutine (watch on the pod name, capture
     termination state on every update — research D4); failure anywhere →
     same cleanup-and-error shape as native (nothing left behind).
2. **`handle.Wait`** blocks on the supervision result: terminal phase or
   Deleted event → map exit (`Code` faithful; Signal inferred 128+n; no
   state observed → `Code: -1`) → reap (delete pod grace-0 idempotently,
   delete Secret, remove staged artifact, close listener) → return status.
   Idempotent via `sync.Once`, the seam's convention.
3. **`handle.Stop`** deletes the pod with `gracePeriodSeconds` = min(stop
   grace, ctx remaining) — TERM at delete, KILL after grace (measured);
   the supervision watch picks up the terminal state before the object
   disappears, so the runner's `Stop → Wait → work.done` path reads a real
   status.
4. **CLI selection** (FR-001): `SOULREALM_BACKEND` gains `k8s`; unknown
   values still fail before any op is published. `SOULREALM_K8S_*` env
   config maps onto the Backend fields; kubeconfig context resolution via
   client-go's standard loading rules.
5. **E2E** (SC-001…SC-005): re-run the M1.1/M1.2 scenario bodies with the
   k8s backend and linux-built artifacts (native control arm asserting
   byte-identical declarations); crash workload asserting `work.abandon` +
   label-sweep empty; the scope probe from research Bar 4 run against the
   suite's operator-mode in-process NATS bound so pods can reach it (host
   alias), asserting in-scope allowed / out-of-scope denied from inside the
   pod; every scenario ends with the zero-leftovers sweep.

## Complexity Tracking

No constitution violations. One deliberate dependency addition (client-go's
68-module graph) is justified in research D1 against its alternative; no
table needed.

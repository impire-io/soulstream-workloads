# Data model: Kubernetes backend

**Feature**: `004-kubernetes-backend` | **Date**: 2026-07-29

No stored data (constitution I). The entities below are runtime state with
bounded lifetimes; everything durable is already ops on the topic.

## Entities

### Backend configuration (node-side, per runner)

| Field | Meaning | Default | Source |
|---|---|---|---|
| `Namespace` | cluster namespace all workload pods/Secrets live in | `default` | `SOULREALM_K8S_NAMESPACE` |
| `Image` | generic runner image (shell + wget + sha256sum + CA trust) | `alpine:3.22` | `SOULREALM_K8S_IMAGE` |
| `HostAlias` | address at which pods reach this node; loopback NATS URLs rewritten to it | — (required when the realm's NATS is loopback) | `SOULREALM_K8S_HOST_ALIAS` |
| `ServeAddr` | bind address for the per-run artifact listener | node-chosen ephemeral port | `SOULREALM_K8S_SERVE_ADDR` |
| kubeconfig/context | cluster access | client-go standard loading rules | `KUBECONFIG` / `SOULREALM_K8S_CONTEXT` |
| client | `kubernetes.Interface` | real clientset | fake in unit tests |

Invariant: none of these may appear in, or be derivable from, a declaration
(constitution III, FR-001).

### Workload pod

One per launch. Name `soulrealm-<workitem-id>` (RFC 1123-sanitized), label
`app.kubernetes.io/managed-by: soulrealm`, `restartPolicy: Never`,
`terminationGracePeriodSeconds` = stop grace (5 s), `emptyDir` scratch at
`/scratch` (workdir), Secret mounted read-only at `/creds`.

**States** (as the supervision watch observes them):

```
created ──▶ Pending ──▶ Running ──▶ Succeeded   (exit 0)
                │           │  └──▶ Failed      (exit N / signal as 128+n)
                │           └─Stop─▶ deleting ──▶ Failed(137/143) or Deleted
                └──(image pull, scheduling)…
   any state ──cluster interference (evict/delete)──▶ Deleted
```

Terminal for supervision: phase `Succeeded`/`Failed`, or a `Deleted` watch
event. After terminal: reap. The pod object never survives its workload.

### Delivered credential (Secret)

Same name as the pod; single key `nats.creds` (the minted JWT+seed formatted
as a creds file). Created before the pod, mounted read-only, deleted at
reap. Never written to host disk. Lifetime ⊆ workload lifetime ⊆ mint TTL.

### Staged artifact

Per-run copy of the resolved artifact bytes: staged under the pod name,
sha256 computed at staging, served by the per-run listener, fetched and
digest-verified in-pod before exec, removed at reap (file + listener).
Precondition: bytes begin with the ELF magic — verified node-side before
any cluster object is created.

### Exit status mapping

| Observed | `backend.ExitStatus` |
|---|---|
| terminated, exitCode 0 | `{Code: 0}` |
| terminated, exitCode N ≤ 128 | `{Code: N}` |
| terminated, Signal field set (rare) | `{Signal: name(sig)}` |
| terminated, exitCode N > 128 | `{Signal: name(N−128)}` — named limitation: a literal exit > 128 is indistinguishable |
| Deleted with no termination state observed | `{Code: -1}` (uncoded failure) |

The mapping feeds `runner.Outcome` unchanged: 0 → `work.done`, anything
else → `work.abandon`; `Stop` → `work.done` regardless (intentional end).

## State transitions owned by the handle

```
Start ──▶ supervising ──(terminal observed)──▶ reaping ──▶ status delivered
   │                                                   (Wait returns; idempotent)
   └─(any Start-phase failure)──▶ rollback: delete Secret/staged bytes; error; nothing behind
Stop ──▶ delete(grace=min(stopGrace, ctx remaining)) ──▶ supervision observes terminal
```

Reap = delete pod (grace 0, idempotent) + delete Secret + remove staged
artifact + close listener. Runs exactly once (`sync.Once`), on every path.

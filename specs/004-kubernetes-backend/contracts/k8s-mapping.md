# Contract: how the Kubernetes backend satisfies the frozen seam

The governing contracts are specs/003's — frozen there, unchanged here:

- [`specs/003-microsandbox-backend/contracts/backend-seam.md`](../../003-microsandbox-backend/contracts/backend-seam.md)
  — the `Backend`/`Handle` behavioral contract every implementation honors.
- [`specs/003-microsandbox-backend/contracts/workload-env.md`](../../003-microsandbox-backend/contracts/workload-env.md)
  — the `SOULREALM_*` env/creds contract a workload sees, byte-for-byte.

This document is the k8s-specific mapping (informative, the msb-mapping
pattern), asserted by `backend/k8s`'s unit tests against the fake clientset.

## Mapping

- **One workload = one pod**, `restartPolicy: Never` (the cluster never
  restarts what the runner supervises), name + Secret
  `soulrealm-<workitem-id>` (RFC 1123-sanitized), label
  `app.kubernetes.io/managed-by: soulrealm`.
- **Credential**: minted creds-file bytes → Secret key `nats.creds` →
  read-only volume at `/creds`; `SOULREALM_NATS_CREDS=/creds/nats.creds`.
  Host disk never touched.
- **Scratch**: `emptyDir` at `/scratch`, the container workdir (native
  parity: cwd = scratch). `LaunchSpec.ScratchDir` donates only the
  work-item id.
- **Artifact**: node-side ELF check → per-run OCI image (artifact layer on
  the CA-trusted `BaseImage`, binary at `/workload` = entrypoint) → pushed
  digest-pinned to the operator's `Registry`, tagged with the work-item id
  → the pod's single container runs it, `command = ["/workload",
  spec.Args…]`. Integrity is the image digest, enforced by the kubelet. The
  declared artifact reference never changes (M1.3's stable-path/
  provisioned-content convention).
- **Env**: exactly the workload-env contract; `SOULREALM_NATS_SERVERS`
  loopback hosts rewritten to the node's `HostAlias` (shared
  `backend/natsurl` helper — same behavior msb's tests pin); non-loopback
  URLs pass through untouched.
- **Wait**: supervision watch (field selector on the pod name) captures
  `ContainerStateTerminated` on every update; terminal phase or Deleted
  event → exit mapping (data-model.md table) → reap → status. Idempotent.
- **Stop**: pod deletion with `gracePeriodSeconds` = min(5 s stop grace,
  ctx remaining): TERM at delete, KILL after grace — the native escalation
  expressed in cluster vocabulary. Stop publishes nothing; the runner owns
  the terminal op.
- **Reap** (every end of life): delete pod (grace 0, idempotent) + delete
  Secret. Zero cluster-side leftovers is asserted e2e by listing the
  managed-by label; per-run images in the registry are operator-retention
  transport cache, not run state.
- **No ops, no control channel**: the backend never touches NATS; the
  supervision watch is read-only cluster state, not a coordination channel.

## Start-failure rollback

Any failure between the ELF check and a running supervision goroutine
unwinds what was created on the cluster (Secret, pod if created) and
returns an error — the runner then publishes `work.open` + `work.abandon`
with no dangling claim, identical to native/msb semantics. An image already
pushed when a later step fails remains in the registry (operator retention
— recorded, not hidden).

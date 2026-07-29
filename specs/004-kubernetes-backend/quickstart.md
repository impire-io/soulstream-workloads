# Quickstart: run the M2.1 slice by hand

**Feature**: `004-kubernetes-backend` — the M1.1 agent, unchanged, as a pod.

## Prerequisites (operator-provided, like `msb` for M1.3)

- A reachable Kubernetes cluster + kubeconfig context, and an OCI registry
  the node can push to and the cluster can pull from. Dev machine:
  `./scripts/kind-registry.sh up` (cluster `kind-soulrealm-k8s` + registry
  `localhost:5001`, the documented kind-with-registry pattern;
  `down` removes both).
- A NATS server the *pods* can reach. Dev machine: bind it to `0.0.0.0` and
  note the address pods see the host at (Docker Desktop:
  `192.168.65.254`) — that address is the host alias below. A routable
  (non-loopback) NATS needs no alias at all.
- Go toolchain (artifact builds are `GOOS=linux GOARCH=<node-arch>
  CGO_ENABLED=0`).

## Run the agent as a pod

```sh
# 1. Build the reference agent for the pod's platform
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/agent-echo ./cmd/agent-echo

# 2. Point the declaration at it (byte-identical shape to M1.1 — no backend field)
#    artifact: file:///tmp/agent-echo

# 3. Select the backend node-side and run — soulrealm assembles the per-run
#    OCI image (artifact on the CA-trusted base), pushes it digest-pinned,
#    and the pod runs it. (Realm identity env — SOULREALM_REALM/PERSONA/
#    REALM_SIGNING_KEY/ROOT_ACCOUNT — as in M1.1.)
SOULREALM_BACKEND=k8s \
SOULREALM_K8S_NAMESPACE=default \
SOULREALM_K8S_REGISTRY=localhost:5001/soulrealm \
SOULREALM_K8S_BASE_IMAGE=alpine:3.22 \
SOULREALM_K8S_HOST_ALIAS=192.168.65.254 \
  soulrealm workload start agent.json
```

Observe on the topic: the persona-attributed turn and
`work.open`/`work.claim`/`work.done` — indistinguishable from the native
run. During the run, `kubectl get pods -l
app.kubernetes.io/managed-by=soulrealm` shows exactly one pod; after it,
none (and no Secret).

## The gate

```sh
make check        # hermetic: fake clientset, in-process NATS, no cluster
make test-k8s     # real cluster: SC-001..SC-004 against kind + zero-leftovers sweep
```

M2.1 exit gate: `make check && make test-k8s` green on the operator's
machine (constitution VI).

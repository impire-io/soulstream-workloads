#!/usr/bin/env bash
# kind-registry.sh up|down — the operator prerequisite for `make test-k8s`
# (specs/004, T002): a kind cluster plus a local OCI registry, wired per the
# documented kind local-registry pattern
# (https://kind.sigs.k8s.io/docs/user/local-registry/).
#
# Name parity (analysis C1): the node pushes to localhost:${REG_PORT} and
# pods reference the SAME name — containerd on every kind node rewrites
# localhost:${REG_PORT} to the registry container via hosts.toml, so one
# reference works on both sides of the wall.
set -euo pipefail

CLUSTER="${SOULREALM_K8S_CLUSTER:-soulrealm-k8s}"
REG_NAME="${SOULREALM_K8S_REG_NAME:-soulrealm-registry}"
REG_PORT="${SOULREALM_K8S_REG_PORT:-5001}"

up() {
  # 1. Local registry container (idempotent).
  if [ "$(docker inspect -f '{{.State.Running}}' "${REG_NAME}" 2>/dev/null || true)" != 'true' ]; then
    docker run -d --restart=always -p "127.0.0.1:${REG_PORT}:5000" \
      --network bridge --name "${REG_NAME}" registry:2
  fi

  # 2. Cluster with the registry config dir enabled (idempotent).
  if ! kind get clusters 2>/dev/null | grep -qx "${CLUSTER}"; then
    cat <<EOF | kind create cluster --name "${CLUSTER}" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry]
    config_path = "/etc/containerd/certs.d"
EOF
  fi

  # 3. Map localhost:${REG_PORT} → the registry container on every node.
  REGISTRY_DIR="/etc/containerd/certs.d/localhost:${REG_PORT}"
  for node in $(kind get nodes --name "${CLUSTER}"); do
    docker exec "${node}" mkdir -p "${REGISTRY_DIR}"
    cat <<EOF | docker exec -i "${node}" cp /dev/stdin "${REGISTRY_DIR}/hosts.toml"
[host."http://${REG_NAME}:5000"]
EOF
  done

  # 4. Registry joins the cluster network (idempotent).
  if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${REG_NAME}")" = 'null' ]; then
    docker network connect "kind" "${REG_NAME}"
  fi

  # 5. Advertise per KEP-1755.
  cat <<EOF | kubectl --context "kind-${CLUSTER}" apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${REG_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

  echo "ready: cluster kind-${CLUSTER}, registry localhost:${REG_PORT}"
}

down() {
  kind delete cluster --name "${CLUSTER}" 2>/dev/null || true
  docker rm -f "${REG_NAME}" >/dev/null 2>&1 || true
  echo "removed: cluster ${CLUSTER}, registry ${REG_NAME}"
}

case "${1:-}" in
  up) up ;;
  down) down ;;
  *) echo "usage: $0 up|down" >&2; exit 2 ;;
esac

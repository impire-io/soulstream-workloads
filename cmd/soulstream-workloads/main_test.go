package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/impire-io/soulstream-workloads/backend/k8s"
	"github.com/impire-io/soulstream-workloads/backend/msb"
	"github.com/impire-io/soulstream-workloads/backend/native"
)

// TestSelectBackend is FR-001: backend choice is node-side, native by
// default, msb opt-in, and anything unknown fails before a single op.
func TestSelectBackend(t *testing.T) {
	if be, err := selectBackend("", ""); err != nil {
		t.Fatalf("default: %v", err)
	} else if _, ok := be.(*native.Backend); !ok {
		t.Fatalf("default backend = %T, want *native.Backend", be)
	}

	if be, err := selectBackend("native", ""); err != nil {
		t.Fatalf("native: %v", err)
	} else if _, ok := be.(*native.Backend); !ok {
		t.Fatalf("native backend = %T, want *native.Backend", be)
	}

	be, err := selectBackend("msb", "debian:12")
	if err != nil {
		t.Fatalf("msb: %v", err)
	}
	mb, ok := be.(*msb.Backend)
	if !ok {
		t.Fatalf("msb backend = %T, want *msb.Backend", be)
	}
	if mb.Image != "debian:12" {
		t.Fatalf("msb image = %q, want the node-side override", mb.Image)
	}

	if _, err := selectBackend("docker", ""); err == nil {
		t.Fatal("unknown backend name did not fail")
	}
}

// TestSelectBackendK8sRequiresRegistry: the k8s backend fails loud, before
// any op and before touching a kubeconfig, when the registry is missing
// (specs/004 T017).
func TestSelectBackendK8sRequiresRegistry(t *testing.T) {
	t.Setenv("SOULSTREAM_K8S_REGISTRY", "")
	if _, err := selectBackend("k8s", ""); err == nil ||
		!strings.Contains(err.Error(), "SOULSTREAM_K8S_REGISTRY") {
		t.Fatalf("want missing-registry error, got %v", err)
	}
}

// TestSelectBackendK8sConfig: with a registry and a minimal kubeconfig, the
// SOULSTREAM_K8S_* env block maps onto the backend's node-side fields.
func TestSelectBackendK8sConfig(t *testing.T) {
	kubeconfig := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(kubeconfig, []byte(`apiVersion: v1
kind: Config
clusters:
- name: c
  cluster: {server: "https://127.0.0.1:1"}
contexts:
- name: ctx
  context: {cluster: c, user: u}
users:
- name: u
  user: {}
current-context: ctx
`), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
	t.Setenv("SOULSTREAM_K8S_REGISTRY", "localhost:5001/soulstream-workloads")
	t.Setenv("SOULSTREAM_K8S_NAMESPACE", "realm-ns")
	t.Setenv("SOULSTREAM_K8S_BASE_IMAGE", "alpine:3.22")
	t.Setenv("SOULSTREAM_K8S_HOST_ALIAS", "192.168.65.254")

	be, err := selectBackend("k8s", "")
	if err != nil {
		t.Fatalf("k8s: %v", err)
	}
	kb, ok := be.(*k8s.Backend)
	if !ok {
		t.Fatalf("k8s backend = %T, want *k8s.Backend", be)
	}
	if kb.Registry != "localhost:5001/soulstream-workloads" || kb.Namespace != "realm-ns" ||
		kb.BaseImage != "alpine:3.22" || kb.HostAlias != "192.168.65.254" || kb.Client == nil {
		t.Fatalf("k8s config not mapped: %+v", kb)
	}
}

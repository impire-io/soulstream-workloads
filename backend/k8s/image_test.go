package k8s

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// testRegistry runs an in-process OCI registry (research D7: hermetic — no
// daemon, no network beyond the loopback httptest listener) and seeds it
// with a one-layer base image standing in for the CA-trusted base.
func testRegistry(t *testing.T) (host, baseRef string, baseLayers int) {
	t.Helper()
	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	host = strings.TrimPrefix(srv.URL, "http://")

	caLayer, err := tarLayer(t, "etc/ssl/certs/ca-certificates.crt", []byte("test trust store"))
	if err != nil {
		t.Fatalf("base layer: %v", err)
	}
	base, err := mutate.AppendLayers(empty.Image, caLayer)
	if err != nil {
		t.Fatalf("base image: %v", err)
	}
	baseRef = host + "/base:latest"
	ref, err := name.ParseReference(baseRef)
	if err != nil {
		t.Fatalf("parse base ref: %v", err)
	}
	if err := remote.Write(ref, base); err != nil {
		t.Fatalf("push base: %v", err)
	}
	return host, baseRef, 1
}

func tarLayer(t *testing.T, path string, content []byte) (v1.Layer, error) {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: path, Mode: 0o644, Size: int64(len(content))}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(content); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	data := buf.Bytes()
	return tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	})
}

func TestPublishLayersArtifactOnBase(t *testing.T) {
	host, baseRef, baseLayers := testRegistry(t)
	p := &registryPublisher{registry: host + "/soulrealm", base: baseRef}
	artifact := append([]byte{0x7f, 'E', 'L', 'F'}, []byte("fake static binary")...)

	ref, err := p.Publish(context.Background(), artifact, "soulrealm-item-1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	wantPrefix := host + "/soulrealm/workloads@sha256:"
	if !strings.HasPrefix(ref, wantPrefix) {
		t.Fatalf("ref = %q, want digest-pinned under %q", ref, wantPrefix)
	}

	parsed, err := name.ParseReference(ref)
	if err != nil {
		t.Fatalf("parse published ref: %v", err)
	}
	img, err := remote.Image(parsed)
	if err != nil {
		t.Fatalf("pull published: %v", err)
	}

	layers, err := img.Layers()
	if err != nil {
		t.Fatalf("layers: %v", err)
	}
	if len(layers) != baseLayers+1 {
		t.Fatalf("layers = %d, want base(%d)+1", len(layers), baseLayers)
	}

	// The appended layer carries exactly /workload 0755 with the artifact bytes.
	rc, err := layers[len(layers)-1].Uncompressed()
	if err != nil {
		t.Fatalf("uncompress artifact layer: %v", err)
	}
	defer func() { _ = rc.Close() }()
	tr := tar.NewReader(rc)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("artifact layer tar: %v", err)
	}
	if hdr.Name != "workload" || hdr.Mode != 0o755 {
		t.Fatalf("artifact entry = %q mode %o, want \"workload\" mode 755", hdr.Name, hdr.Mode)
	}
	got, _ := io.ReadAll(tr)
	if !bytes.Equal(got, artifact) {
		t.Fatal("artifact bytes changed in transit")
	}

	cf, err := img.ConfigFile()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if len(cf.Config.Entrypoint) != 1 || cf.Config.Entrypoint[0] != guestWorkload {
		t.Fatalf("entrypoint = %v, want [%s]", cf.Config.Entrypoint, guestWorkload)
	}
	if len(cf.Config.Cmd) != 0 {
		t.Fatalf("cmd = %v, want empty (pod command is authoritative)", cf.Config.Cmd)
	}

	// The work-item tag names the same manifest the digest pins.
	tagged, err := remote.Image(mustRef(t, host+"/soulrealm/workloads:soulrealm-item-1"))
	if err != nil {
		t.Fatalf("pull by tag: %v", err)
	}
	td, _ := tagged.Digest()
	if !strings.HasSuffix(ref, td.String()) {
		t.Fatalf("tag digest %s does not match pinned ref %s", td, ref)
	}
}

func mustRef(t *testing.T, s string) name.Reference {
	t.Helper()
	r, err := name.ParseReference(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return r
}

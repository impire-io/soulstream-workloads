package k8s

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// registryPublisher is the production ImagePublisher — the artifact channel
// decided in specs/004 research D2: the resolved artifact bytes become one
// layer on the CA-trusted base image, entrypoint /workload, pushed
// digest-pinned to the operator's registry so the kubelet pulls it exactly
// as it pulls any image. Assembly is pure Go (go-containerregistry) — no
// builder daemon. Push credentials come from the operator's standard
// docker-config keychain; localhost registries go over plain HTTP (the
// kind-with-registry pattern).
type registryPublisher struct {
	registry string // repository prefix, e.g. "localhost:5001/soulstream-workloads"
	base     string // CA-trusted base image reference
}

// Publish layers the artifact onto the base, pushes
// <registry>/workloads:<tag>, and returns the digest-pinned reference the
// pod spec uses (integrity enforced by the kubelet).
func (p *registryPublisher) Publish(ctx context.Context, artifact []byte, tag string) (string, error) {
	baseRef, err := name.ParseReference(p.base)
	if err != nil {
		return "", fmt.Errorf("parse base image %q: %w", p.base, err)
	}
	base, err := remote.Image(baseRef,
		remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return "", fmt.Errorf("pull base image %q: %w", p.base, err)
	}

	layer, err := workloadLayer(artifact)
	if err != nil {
		return "", err
	}
	img, err := mutate.AppendLayers(base, layer)
	if err != nil {
		return "", fmt.Errorf("append artifact layer: %w", err)
	}
	cf, err := img.ConfigFile()
	if err != nil {
		return "", fmt.Errorf("read image config: %w", err)
	}
	cfg := cf.Config
	cfg.Entrypoint = []string{guestWorkload}
	cfg.Cmd = nil
	img, err = mutate.Config(img, cfg)
	if err != nil {
		return "", fmt.Errorf("set entrypoint: %w", err)
	}

	dst := p.registry + "/workloads:" + tag
	dstRef, err := name.ParseReference(dst)
	if err != nil {
		return "", fmt.Errorf("parse target %q: %w", dst, err)
	}
	if err := remote.Write(dstRef, img,
		remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
		return "", fmt.Errorf("push %q: %w", dst, err)
	}
	dig, err := img.Digest()
	if err != nil {
		return "", fmt.Errorf("image digest: %w", err)
	}
	return p.registry + "/workloads@" + dig.String(), nil
}

// workloadLayer wraps the artifact bytes in a single-file tar layer:
// /workload, mode 0755.
func workloadLayer(artifact []byte) (v1.Layer, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: "workload",
		Mode: 0o755,
		Size: int64(len(artifact)),
	}); err != nil {
		return nil, fmt.Errorf("artifact layer header: %w", err)
	}
	if _, err := tw.Write(artifact); err != nil {
		return nil, fmt.Errorf("artifact layer content: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("artifact layer close: %w", err)
	}
	data := buf.Bytes()
	return tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	})
}

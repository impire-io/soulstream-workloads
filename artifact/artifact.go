// Package artifact fetches record-form artefacts: the resolution behind a
// declaration's soulstream://<topic-path>/<artefact-name> reference (hq design
// 0005 §2). An artefact is a stage-1 lineage of whole-file revisions on a
// topic; this package materialises the lineage TIP — the current revision —
// digest-checked against the record, and never holds a durable copy: the
// runner writes it into a run's scratch dir (reaped with the run), the wake
// engine keeps instructions in memory per wake. Reads run on the caller's own
// connection (the runtime-side-reads decision, specs/009): no scope widening,
// no consumer state — the record is the source every single time.
package artifact

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/declaration"
)

// Fetch materialises the artefact lineage named ref (a display name, a lineage
// root op-id, or any revision's op-id) in topicPath and returns the tip's
// bytes, digest-checked. A missing lineage, an ambiguous name, or a digest
// mismatch refuses — never a silent fallback.
func Fetch(ctx context.Context, c *realm.Client, topicPath, ref string) ([]byte, topic.Artefact, error) {
	view, err := topic.Open(c, topicPath).Materialise(ctx)
	if err != nil {
		return nil, topic.Artefact{}, fmt.Errorf("artifact: materialise %s: %w", topicPath, err)
	}
	art, err := topic.FindArtefact(view, ref)
	if err != nil {
		return nil, topic.Artefact{}, fmt.Errorf("artifact: %w", err)
	}
	data, err := topic.GetAttachment(ctx, c, art.Tip.Object)
	if err != nil {
		return nil, topic.Artefact{}, fmt.Errorf("artifact: %w", err)
	}
	if !topic.VerifyDigest(data, art.Tip.Digest) {
		return nil, topic.Artefact{}, fmt.Errorf(
			"artifact: %q tip %s failed its digest check (%s) — the stored bytes are not the recorded bytes",
			ref, art.Tip.OpID, art.Tip.Digest)
	}
	return data, art, nil
}

// Resolver resolves a declaration's artifact to a local executable path: a
// file:// artifact is its host path (unchanged, no client needed); a
// soulstream:// artifact is fetched from the record and written into the
// run's scratch dir — executable, digest-checked, gone when the run's scratch
// is reaped. It satisfies the runner's ArtifactSource seam.
type Resolver struct {
	Client *realm.Client // required only for soulstream:// artifacts
}

// Resolve returns the local path the backend launches.
func (r *Resolver) Resolve(ctx context.Context, d declaration.Declaration, scratchDir string) (string, error) {
	ref, err := d.ArtifactRef()
	if err != nil {
		return "", err
	}
	switch ref.Scheme {
	case declaration.SchemeFile:
		return ref.Path, nil
	case declaration.SchemeSoulstream:
		if r == nil || r.Client == nil {
			return "", fmt.Errorf("artifact: resolving %s needs a realm client", d.Artifact)
		}
		data, _, err := Fetch(ctx, r.Client, ref.Topic, ref.Name)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(scratchDir, 0o755); err != nil {
			return "", fmt.Errorf("artifact: scratch dir: %w", err)
		}
		path := filepath.Join(scratchDir, ref.Name)
		if err := os.WriteFile(path, data, 0o700); err != nil {
			return "", fmt.Errorf("artifact: write %s: %w", path, err)
		}
		return path, nil
	default:
		return "", fmt.Errorf("artifact: scheme %q is not resolvable", ref.Scheme)
	}
}

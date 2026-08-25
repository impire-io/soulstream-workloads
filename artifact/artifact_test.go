package artifact

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/internal/natstest"
)

func startRealm(t *testing.T) (*realm.Client, string) {
	t.Helper()
	url, shutdown := natstest.StartJetStream(t)
	t.Cleanup(shutdown)
	ctx := context.Background()
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	c, err := realm.NewClient(ctx, nc, realm.Config{Realm: "test-realm", Persona: "owner"})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if _, err := c.Provision(ctx); err != nil {
		t.Fatalf("provision: %v", err)
	}
	h, err := topic.StartTopic(ctx, c, topic.StartTopicInput{Name: "artefacts"})
	if err != nil {
		t.Fatalf("start topic: %v", err)
	}
	return c, h.Path()
}

// Fetch returns the lineage TIP, digest-checked: a revision supersedes the
// root, and the returned bytes are the revision's.
func TestFetchReturnsRevisedTip(t *testing.T) {
	c, path := startRealm(t)
	ctx := context.Background()
	h := topic.Open(c, path)

	rootOp, err := h.Attach(ctx, "agent.md", "text/markdown", []byte("v1: be brief"), "")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if _, err := h.Revise(ctx, "agent.md", "text/markdown", []byte("v2: be thorough"), rootOp); err != nil {
		t.Fatalf("revise: %v", err)
	}

	data, art, err := Fetch(ctx, c, path, "agent.md")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(data) != "v2: be thorough" {
		t.Fatalf("tip bytes = %q, want the revision", data)
	}
	if art.Root != rootOp || len(art.Revisions) != 2 {
		t.Fatalf("lineage = root %s revisions %d, want root %s revisions 2", art.Root, len(art.Revisions), rootOp)
	}
}

// A tampered object store (bytes no longer matching the recorded digest)
// refuses — never a silent serve.
func TestFetchRefusesDigestMismatch(t *testing.T) {
	c, path := startRealm(t)
	ctx := context.Background()
	h := topic.Open(c, path)

	if _, err := h.Attach(ctx, "agent.bin", "application/octet-stream", []byte("honest bytes"), ""); err != nil {
		t.Fatalf("attach: %v", err)
	}
	_, art, err := Fetch(ctx, c, path, "agent.bin")
	if err != nil {
		t.Fatalf("Fetch pre-tamper: %v", err)
	}
	store, err := c.JetStream().ObjectStore(ctx, realm.ObjectBucket)
	if err != nil {
		t.Fatalf("object store: %v", err)
	}
	if _, err := store.PutBytes(ctx, art.Tip.Object, []byte("tampered bytes")); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	if _, _, err := Fetch(ctx, c, path, "agent.bin"); err == nil ||
		!strings.Contains(err.Error(), "digest") {
		t.Fatalf("Fetch after tamper = %v, want a digest refusal", err)
	}
}

// A missing lineage refuses with the topic and name in the error.
func TestFetchRefusesMissing(t *testing.T) {
	c, path := startRealm(t)
	if _, _, err := Fetch(context.Background(), c, path, "nothing.md"); err == nil {
		t.Fatal("Fetch of a missing artefact must refuse")
	}
}

// The resolver: file:// is the host path with no client; soulstream:// is a
// digest-checked scratch copy, executable, inside the given scratch dir.
func TestResolverBothSchemes(t *testing.T) {
	c, path := startRealm(t)
	ctx := context.Background()

	fileDecl := declaration.Declaration{
		Role: declaration.RoleAgent, Lifecycle: declaration.LifecycleService,
		Persona: "clerk", Topic: path, Artifact: "file:///opt/agents/clerk",
	}
	var noClient *Resolver
	got, err := noClient.Resolve(ctx, fileDecl, t.TempDir())
	if err != nil || got != "/opt/agents/clerk" {
		t.Fatalf("file resolve = %q, %v", got, err)
	}

	if _, err := topic.Open(c, path).Attach(ctx, "clerk-bin", "application/octet-stream",
		[]byte("#!/bin/sh\necho ok\n"), ""); err != nil {
		t.Fatalf("attach: %v", err)
	}
	recDecl := fileDecl
	recDecl.Artifact = "soulstream://" + path + "/clerk-bin"
	scratch := t.TempDir()
	r := &Resolver{Client: c}
	resolved, err := r.Resolve(ctx, recDecl, scratch)
	if err != nil {
		t.Fatalf("record resolve: %v", err)
	}
	if filepath.Dir(resolved) != scratch {
		t.Fatalf("resolved %q outside scratch %q", resolved, scratch)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("resolved artifact is not executable: %v", info.Mode())
	}
	data, _ := os.ReadFile(resolved)
	if string(data) != "#!/bin/sh\necho ok\n" {
		t.Fatalf("resolved bytes = %q", data)
	}

	// A record-form artifact without a client refuses before anything runs.
	if _, err := noClient.Resolve(ctx, recDecl, t.TempDir()); err == nil {
		t.Fatal("record-form resolve without a client must refuse")
	}
}

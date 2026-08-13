package native

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nkeys"

	"github.com/impire-io/soulstream-workloads/backend"
	"github.com/impire-io/soulstream-workloads/minter"
)

func testCred(t *testing.T) minter.PersonaScopedCredential {
	t.Helper()
	acc, _ := nkeys.CreateAccount()
	accPub, _ := acc.PublicKey()
	seed, _ := acc.Seed()
	m, err := minter.NewSigningKeyMinter(seed, accPub, []string{"nats://127.0.0.1:4222"})
	if err != nil {
		t.Fatalf("minter: %v", err)
	}
	cred, err := m.Mint(minter.Scope{Persona: "researcher", Topic: "t-ab12"}, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return cred
}

func spec(t *testing.T, artifact string, args ...string) backend.LaunchSpec {
	return backend.LaunchSpec{
		Artifact:   artifact,
		Args:       args,
		Cred:       testCred(t),
		Realm:      "acme",
		Topic:      "t-ab12",
		ScratchDir: t.TempDir() + "/wl",
	}
}

func TestCleanExit(t *testing.T) {
	h, err := New().Start(context.Background(), spec(t, "/bin/sh", "-c", "exit 0"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if st := h.Wait(); st.Code != 0 || st.Signalled() {
		t.Fatalf("status = %+v, want code 0", st)
	}
}

func TestNonzeroExit(t *testing.T) {
	h, err := New().Start(context.Background(), spec(t, "/bin/sh", "-c", "exit 3"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if st := h.Wait(); st.Code != 3 {
		t.Fatalf("status = %+v, want code 3", st)
	}
}

func TestSignalledExit(t *testing.T) {
	h, err := New().Start(context.Background(), spec(t, "/bin/sh", "-c", "sleep 30"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if st := h.Wait(); !st.Signalled() {
		t.Fatalf("status = %+v, want a signal", st)
	}
}

// The child must see its own scoped identity but NOT soulstream-workloads's own env —
// proving the clean-env isolation (constitution II). The parent sets a fake
// soulstream-workloads secret; the child exits 0 only if creds+topic are present and the
// secret is absent.
func TestCleanEnvIsolation(t *testing.T) {
	t.Setenv("SOULSTREAM_TEST_SECRET", "the-signing-key")
	script := `[ -f "$SOULSTREAM_NATS_CREDS" ] && [ "$SOULSTREAM_TOPIC" = "t-ab12" ] && ` +
		`[ "$SOULSTREAM_PERSONA" = "researcher" ] && [ -z "$SOULSTREAM_TEST_SECRET" ]`
	h, err := New().Start(context.Background(), spec(t, "/bin/sh", "-c", script))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if st := h.Wait(); st.Code != 0 || st.Signalled() {
		t.Fatalf("isolation check failed (status %+v): creds/topic/persona missing or the soulstream-workloads secret leaked", st)
	}
}

func TestScratchReapedOnExit(t *testing.T) {
	s := spec(t, "/bin/sh", "-c", "exit 0")
	h, err := New().Start(context.Background(), s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	h.Wait()
	if _, err := os.Stat(s.ScratchDir); !os.IsNotExist(err) {
		t.Fatalf("scratch dir %q not reaped (err=%v)", s.ScratchDir, err)
	}
}

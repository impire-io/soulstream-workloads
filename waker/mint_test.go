package waker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	natsjwt "github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// fakeMinter signs a real (throwaway-account) user JWT — the creds assembly
// decodes what it embeds, so the stand-in must be wire-shaped.
type fakeMinter struct {
	gotRole string
	gotUser string
	gotTTL  time.Duration
}

func (f *fakeMinter) MintEphemeral(role, user, userPublicKey string, ttl time.Duration, _ []string) (string, error) {
	f.gotRole, f.gotUser, f.gotTTL = role, user, ttl
	if userPublicKey == "" || !strings.HasPrefix(userPublicKey, "U") {
		return "", os.ErrInvalid
	}
	acc, err := nkeys.CreateAccount()
	if err != nil {
		return "", err
	}
	uc := natsjwt.NewUserClaims(userPublicKey)
	uc.Name = user
	return uc.Encode(acc)
}

// The ephemeral lane's run credential: minted against the declared role with
// a locally generated key (the seed never reaches the minter), assembled into
// a 0600 creds file inside the run directory — scratch, reaped with the run.
func TestMintRunCredential(t *testing.T) {
	m := &fakeMinter{}
	runDir := filepath.Join(t.TempDir(), "run")
	lane := &EphemeralLane{Role: "realm", TTL: Duration(90 * time.Second)}

	path, err := mintRunCredential(m, lane, "clerk", runDir)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if m.gotRole != "realm" || m.gotUser != "clerk" || m.gotTTL != 90*time.Second {
		t.Fatalf("minter saw role=%q user=%q ttl=%v", m.gotRole, m.gotUser, m.gotTTL)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("creds file mode = %v, err = %v, want 0600", info.Mode().Perm(), err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read creds: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "BEGIN NATS USER JWT") || !strings.Contains(s, "SU") {
		t.Fatalf("creds file missing JWT or locally generated seed:\n%s", s)
	}
	if !strings.HasPrefix(filepath.Dir(path), filepath.Dir(runDir)) {
		t.Fatalf("creds landed outside the run dir: %s", path)
	}
}

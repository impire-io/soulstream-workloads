package minter

import (
	"strings"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"

	"github.com/impire-io/soulstream-workloads/declaration"
)

func newScopedMinter(t *testing.T) (*ScopedSigningKeyMinter, string) {
	t.Helper()
	role, _ := nkeys.CreateAccount()
	seed, _ := role.Seed()
	rolePub, _ := role.PublicKey()
	root, _ := nkeys.CreateAccount()
	rootPub, _ := root.PublicKey()
	m, err := NewScopedSigningKeyMinter(seed, rootPub, []string{"nats://127.0.0.1:4222"})
	if err != nil {
		t.Fatalf("scoped minter: %v", err)
	}
	_ = rolePub
	return m, rootPub
}

// TestScopedMintClaimShape: the minted JWT is the D28 shape — no
// permissions of its own, the scope's tags in the claims, TTL bound, the
// account membership declared. The account template is the entire policy.
func TestScopedMintClaimShape(t *testing.T) {
	m, rootPub := newScopedMinter(t)
	cred, err := m.Mint(Scope{
		Persona: "sprite", Topic: "acme-team.q2-ab12",
		Capabilities: &declaration.Capabilities{Role: "agent", Tools: []string{"toola", "toolb"}},
	}, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	uc, err := jwt.DecodeUserClaims(cred.UserJWT)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !uc.HasEmptyPermissions() {
		t.Fatalf("scoped user carries its own permissions: %+v", uc.Permissions)
	}
	if uc.Name != "sprite" || uc.IssuerAccount != rootPub {
		t.Fatalf("claims name=%q issuer-account=%q", uc.Name, uc.IssuerAccount)
	}
	for _, want := range []string{"persona:sprite", "topic:acme-team.q2-ab12", "tool:toola", "tool:toolb"} {
		if !uc.Tags.Contains(want) {
			t.Fatalf("tags %v missing %q", uc.Tags, want)
		}
	}
	if uc.Expires == 0 {
		t.Fatal("scoped user carries no expiry")
	}
}

// TestScopedMintRefusals: the capability-less scope belongs to the plain
// lane; bad selectors refuse before any signing happens.
func TestScopedMintRefusals(t *testing.T) {
	m, _ := newScopedMinter(t)
	if _, err := m.Mint(Scope{Persona: "sprite", Topic: "t-ab12"}, time.Hour); err == nil ||
		!strings.Contains(err.Error(), "capability-less") {
		t.Fatalf("capability-less scope accepted: %v", err)
	}
	if _, err := m.Mint(Scope{
		Persona: "sprite", Topic: "t-ab12",
		Capabilities: &declaration.Capabilities{Role: "agent", Tools: []string{">"}},
	}, time.Hour); err == nil {
		t.Fatal("subject-grammar tool accepted")
	}
	if _, err := m.Mint(Scope{
		Persona: "sprite", Topic: "t-ab12",
		Capabilities: &declaration.Capabilities{Role: "agent"},
	}, 0); err == nil {
		t.Fatal("zero ttl accepted")
	}
}

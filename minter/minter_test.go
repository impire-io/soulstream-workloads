package minter

import (
	"testing"
	"time"

	"github.com/impire-io/soulstream-core/topic"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"

	"github.com/impire-io/soulstream-workloads/declaration"
)

func TestPermissionSetScope(t *testing.T) {
	s := Scope{Persona: "researcher", Topic: "acme-team.q2-planning-ab12"}
	ps := s.PermissionSet()

	ops := topic.OpsSubject(s.Topic)
	wantPub := map[string]bool{ops: true, notifyWildcard: true, svcWildcard: true,
		inboxWildcard: true, jsAPIInfo: true}
	if len(ps.Pub) != len(wantPub) {
		t.Fatalf("pub allow = %v, want %d entries", ps.Pub, len(wantPub))
	}
	for _, p := range ps.Pub {
		if !wantPub[p] {
			t.Fatalf("unexpected pub allow %q", p)
		}
	}

	wantSub := map[string]bool{
		ops:                       true,
		topic.InfoSubjectWildcard: true,
		notifyPrefix + s.Persona:  true,
		inboxWildcard:             true,
	}
	for _, p := range ps.Sub {
		if !wantSub[p] {
			t.Fatalf("unexpected sub allow %q", p)
		}
	}

	// SC-003 basis: the scope must NOT allow an unrelated subject.
	for _, p := range ps.Pub {
		if p == "SOMETHING.ELSE" || p == ">" {
			t.Fatalf("scope leaks a broad/unrelated pub permission %q", p)
		}
	}
}

func TestToolScopeServesOnly(t *testing.T) {
	s := Scope{Role: declaration.RoleTool, Persona: "uppercase", Topic: "t-ab12"}
	ps := s.PermissionSet()

	// A tool subscribes only its own service subject (+ inbox) and publishes
	// only replies. It must NOT be able to publish topic ops or call tools.
	svc := ServiceSubject("uppercase")
	wantSub := map[string]bool{svc: true, inboxWildcard: true}
	if len(ps.Sub) != len(wantSub) {
		t.Fatalf("tool sub = %v, want %v", ps.Sub, wantSub)
	}
	for _, p := range ps.Sub {
		if !wantSub[p] {
			t.Fatalf("unexpected tool sub %q", p)
		}
	}
	for _, p := range ps.Pub {
		if p != inboxWildcard {
			t.Fatalf("tool must only publish replies, got pub %q", p)
		}
	}
}

func TestPermissionSetPure(t *testing.T) {
	s := Scope{Persona: "p", Topic: "t-ab12"}
	a, b := s.PermissionSet(), s.PermissionSet()
	if len(a.Pub) != len(b.Pub) || len(a.Sub) != len(b.Sub) {
		t.Fatal("PermissionSet is not deterministic")
	}
}

func testMinter(t *testing.T) (*SigningKeyMinter, string, string) {
	t.Helper()
	account, err := nkeys.CreateAccount()
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	accountPub, _ := account.PublicKey()

	signing, err := nkeys.CreateAccount()
	if err != nil {
		t.Fatalf("create signing key: %v", err)
	}
	signingSeed, _ := signing.Seed()
	signingPub, _ := signing.PublicKey()

	m, err := NewSigningKeyMinter(signingSeed, accountPub, []string{"nats://127.0.0.1:4222"})
	if err != nil {
		t.Fatalf("NewSigningKeyMinter: %v", err)
	}
	return m, accountPub, signingPub
}

func TestMintProducesScopedUser(t *testing.T) {
	m, accountPub, signingPub := testMinter(t)

	s := Scope{Persona: "researcher", Topic: "acme-team.q2-planning-ab12"}
	cred, err := m.Mint(s, time.Hour)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	claims, err := jwt.DecodeUserClaims(cred.UserJWT)
	if err != nil {
		t.Fatalf("DecodeUserClaims: %v", err)
	}
	if claims.Name != "researcher" {
		t.Fatalf("Name = %q, want researcher", claims.Name)
	}
	if claims.IssuerAccount != accountPub {
		t.Fatalf("IssuerAccount = %q, want %q", claims.IssuerAccount, accountPub)
	}
	if claims.Issuer != signingPub {
		t.Fatalf("Issuer = %q, want signing key %q", claims.Issuer, signingPub)
	}
	if claims.Expires == 0 || time.Unix(claims.Expires, 0).Before(time.Now()) {
		t.Fatalf("Expires not in the future: %d", claims.Expires)
	}

	// permissions match the scope
	want := s.PermissionSet()
	if !sameSet([]string(claims.Pub.Allow), want.Pub) {
		t.Fatalf("pub allow = %v, want %v", claims.Pub.Allow, want.Pub)
	}
	if !sameSet([]string(claims.Sub.Allow), want.Sub) {
		t.Fatalf("sub allow = %v, want %v", claims.Sub.Allow, want.Sub)
	}

	// the seed is a usable user key
	ukp, err := nkeys.FromSeed(cred.UserSeed)
	if err != nil {
		t.Fatalf("user seed unusable: %v", err)
	}
	pub, _ := ukp.PublicKey()
	if !nkeys.IsValidPublicUserKey(pub) {
		t.Fatalf("minted seed is not a user key")
	}
	if claims.Subject != pub {
		t.Fatalf("JWT subject %q != seed public %q", claims.Subject, pub)
	}
}

func TestMintValidation(t *testing.T) {
	m, _, _ := testMinter(t)
	if _, err := m.Mint(Scope{Persona: "", Topic: "t"}, time.Hour); err == nil {
		t.Fatal("expected error on empty persona")
	}
	if _, err := m.Mint(Scope{Persona: "p", Topic: "t"}, 0); err == nil {
		t.Fatal("expected error on non-positive ttl")
	}
}

func TestNewSigningKeyMinterValidation(t *testing.T) {
	account, _ := nkeys.CreateAccount()
	accountPub, _ := account.PublicKey()
	seed, _ := account.Seed()

	if _, err := NewSigningKeyMinter([]byte("not-a-seed"), accountPub, []string{"nats://x"}); err == nil {
		t.Fatal("expected error on bad signing seed")
	}
	if _, err := NewSigningKeyMinter(seed, "not-an-account-key", []string{"nats://x"}); err == nil {
		t.Fatal("expected error on bad root account key")
	}
	if _, err := NewSigningKeyMinter(seed, accountPub, nil); err == nil {
		t.Fatal("expected error on empty servers")
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int, len(a))
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		m[s]--
		if m[s] < 0 {
			return false
		}
	}
	return true
}

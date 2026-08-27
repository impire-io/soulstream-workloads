package minter

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulstream-core/topic"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"

	"github.com/impire-io/soulstream-workloads/declaration"
)

// TestPermissionSetCapabilityLessGolden pins the capability-less derivation
// byte-for-byte (spec 010 SC-003): capability-minting must be additive — a
// declaration without capabilities mints exactly the credential it minted
// before the feature, list order included.
func TestPermissionSetCapabilityLessGolden(t *testing.T) {
	agent := Scope{Persona: "researcher", Topic: "acme-team.q2-planning-ab12"}
	ops := topic.OpsSubject(agent.Topic)
	ps := agent.PermissionSet()
	wantPub := []string{ops, "SOULSTREAM.PERSONA.NOTIFY.*", "SOULSTREAM.SVC.>", "_INBOX.>", "$JS.API.INFO"}
	wantSub := []string{ops, "SOULSTREAM.TOPICS.INFO.>", "SOULSTREAM.PERSONA.NOTIFY.researcher", "_INBOX.>"}
	if !reflect.DeepEqual(ps.Pub, wantPub) {
		t.Fatalf("agent pub = %v, want %v (byte-identical)", ps.Pub, wantPub)
	}
	if !reflect.DeepEqual(ps.Sub, wantSub) {
		t.Fatalf("agent sub = %v, want %v (byte-identical)", ps.Sub, wantSub)
	}

	tool := Scope{Role: declaration.RoleTool, Persona: "uppercase", Topic: "t-ab12"}
	tps := tool.PermissionSet()
	if !reflect.DeepEqual(tps.Pub, []string{"_INBOX.>"}) {
		t.Fatalf("tool pub = %v, want [_INBOX.>]", tps.Pub)
	}
	if !reflect.DeepEqual(tps.Sub, []string{"SOULSTREAM.SVC.uppercase", "_INBOX.>"}) {
		t.Fatalf("tool sub = %v", tps.Sub)
	}
}

// TestPermissionSetNarrowsToDeclaredTools: capabilities replace the tool
// wildcard with exactly the declared tools, in order (FR-002).
func TestPermissionSetNarrowsToDeclaredTools(t *testing.T) {
	s := Scope{
		Persona: "sprite", Topic: "t-ab12",
		Capabilities: &declaration.Capabilities{Role: "agent", Tools: []string{"toola", "toolb"}},
	}
	ops := topic.OpsSubject(s.Topic)
	ps := s.PermissionSet()
	wantPub := []string{ops, "SOULSTREAM.PERSONA.NOTIFY.*", "SOULSTREAM.SVC.toola", "SOULSTREAM.SVC.toolb", "_INBOX.>", "$JS.API.INFO"}
	if !reflect.DeepEqual(ps.Pub, wantPub) {
		t.Fatalf("pub = %v, want %v", ps.Pub, wantPub)
	}
	for _, p := range ps.Pub {
		if p == "SOULSTREAM.SVC.>" {
			t.Fatal("the tool wildcard survived a capability scope")
		}
	}
}

// TestPermissionSetZeroToolsGrantsNoToolSubject: role-only capabilities
// reach no tool subject at all (FR-002, SC-002's pure half).
func TestPermissionSetZeroToolsGrantsNoToolSubject(t *testing.T) {
	s := Scope{
		Persona: "sprite", Topic: "t-ab12",
		Capabilities: &declaration.Capabilities{Role: "agent"},
	}
	ops := topic.OpsSubject(s.Topic)
	wantPub := []string{ops, "SOULSTREAM.PERSONA.NOTIFY.*", "_INBOX.>", "$JS.API.INFO"}
	if ps := s.PermissionSet(); !reflect.DeepEqual(ps.Pub, wantPub) {
		t.Fatalf("pub = %v, want %v", ps.Pub, wantPub)
	}
}

// TestMintTags renders the canonical tag list (FR-003) and nothing for a
// capability-less scope.
func TestMintTags(t *testing.T) {
	bare := Scope{Persona: "sprite", Topic: "t-ab12"}
	if tags, err := bare.MintTags(); err != nil || tags != nil {
		t.Fatalf("capability-less tags = %v, %v; want nil, nil", tags, err)
	}

	s := Scope{
		Persona: "sprite", Topic: "acme-team.q2-ab12",
		Capabilities: &declaration.Capabilities{Role: "agent", Tools: []string{"toola", "toolb"}},
	}
	tags, err := s.MintTags()
	if err != nil {
		t.Fatalf("tags: %v", err)
	}
	want := []string{"persona:sprite", "topic:acme-team.q2-ab12", "tool:toola", "tool:toolb"}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("tags = %v, want %v", tags, want)
	}
}

// TestMintTagsRefusesBadValues: no value that could alter subject grammar
// may ever leave the renderer (FR-004).
func TestMintTagsRefusesBadValues(t *testing.T) {
	cases := []struct {
		name string
		s    Scope
	}{
		{"tool with wildcard", Scope{Persona: "p", Topic: "t-ab12",
			Capabilities: &declaration.Capabilities{Role: "agent", Tools: []string{">"}}}},
		{"tool with dot", Scope{Persona: "p", Topic: "t-ab12",
			Capabilities: &declaration.Capabilities{Role: "agent", Tools: []string{"a.b"}}}},
		{"invalid role", Scope{Persona: "p", Topic: "t-ab12",
			Capabilities: &declaration.Capabilities{Role: "Bad Role"}}},
		{"duplicate tool", Scope{Persona: "p", Topic: "t-ab12",
			Capabilities: &declaration.Capabilities{Role: "agent", Tools: []string{"a", "a"}}}},
		{"invalid persona", Scope{Persona: "NOT VALID", Topic: "t-ab12",
			Capabilities: &declaration.Capabilities{Role: "agent"}}},
		{"invalid topic segment", Scope{Persona: "p", Topic: "t..x",
			Capabilities: &declaration.Capabilities{Role: "agent"}}},
	}
	for _, c := range cases {
		if _, err := c.s.MintTags(); err == nil {
			t.Fatalf("%s: MintTags accepted a bad value", c.name)
		}
	}
}

// TestMintHonorsCapabilities: the local minter narrows (the minted JWT
// carries exactly the declared tools) and refuses invalid selectors at its
// own boundary (FR-005/FR-006).
func TestMintHonorsCapabilities(t *testing.T) {
	signing, _ := nkeys.CreateAccount()
	seed, _ := signing.Seed()
	root, _ := nkeys.CreateAccount()
	rpub, _ := root.PublicKey()
	m, err := NewSigningKeyMinter(seed, rpub, []string{"nats://127.0.0.1:4222"})
	if err != nil {
		t.Fatalf("minter: %v", err)
	}

	cred, err := m.Mint(Scope{
		Persona: "sprite", Topic: "t-ab12",
		Capabilities: &declaration.Capabilities{Role: "agent", Tools: []string{"toola"}},
	}, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	uc, err := jwt.DecodeUserClaims(cred.UserJWT)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var hasTool, hasWildcard bool
	for _, p := range uc.Pub.Allow {
		if p == "SOULSTREAM.SVC.toola" {
			hasTool = true
		}
		if p == "SOULSTREAM.SVC.>" {
			hasWildcard = true
		}
	}
	if !hasTool || hasWildcard {
		t.Fatalf("minted pub allow = %v: want the declared tool and no wildcard", uc.Pub.Allow)
	}

	// The mint boundary re-refuses what a declaration would have refused.
	if _, err := m.Mint(Scope{
		Role: declaration.RoleTool, Persona: "uppercase", Topic: "t-ab12",
		Capabilities: &declaration.Capabilities{Role: "agent"},
	}, time.Hour); err == nil || !strings.Contains(err.Error(), "agent-only") {
		t.Fatalf("tool-role capabilities accepted: %v", err)
	}
	if _, err := m.Mint(Scope{
		Persona: "sprite", Topic: "t-ab12",
		Capabilities: &declaration.Capabilities{Role: "agent", Tools: []string{"*"}},
	}, time.Hour); err == nil {
		t.Fatal("subject-grammar tool name accepted at mint")
	}
}

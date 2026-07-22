package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream/topic"

	"github.com/impire-io/soulrealm/declaration"
	"github.com/impire-io/soulrealm/internal/natstest"
	"github.com/impire-io/soulrealm/minter"
)

// TestMintedCredentialScopeEnforced is SC-003: against an operator-mode server
// that actually enforces user JWT permissions, a minted credential may publish
// on its topic's ops subject but is DENIED any subject outside its scope.
func TestMintedCredentialScopeEnforced(t *testing.T) {
	op := natstest.StartOperator(t)
	defer op.Shutdown()

	m, err := minter.NewSigningKeyMinter(op.AccountSigningSeed, op.RootAccountKey, []string{op.URL})
	if err != nil {
		t.Fatalf("minter: %v", err)
	}

	topicPath := "planning-ab12"
	cred, err := m.Mint(minter.Scope{Persona: "researcher", Topic: topicPath}, time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	permErr := make(chan error, 8)
	nc, err := nats.Connect(op.URL,
		nats.UserCredentials(writeCreds(t, cred)),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, e error) { permErr <- e }),
	)
	if err != nil {
		t.Fatalf("connect with minted creds: %v", err)
	}
	defer nc.Close()

	// In scope: publishing on the topic's ops subject is allowed.
	if err := nc.Publish(topic.OpsSubject(topicPath), []byte("op")); err != nil {
		t.Fatalf("in-scope publish call: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Out of scope: publishing anywhere else must be denied by the server.
	if err := nc.Publish("SOMETHING.ELSE", []byte("nope")); err != nil {
		t.Fatalf("out-of-scope publish call: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	select {
	case e := <-permErr:
		if !strings.Contains(strings.ToLower(e.Error()), "permission") {
			t.Fatalf("expected a permissions violation, got: %v", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SC-003: out-of-scope publish was NOT denied by the server")
	}
}

// TestToolCredentialScopeEnforced is the M1.2 half of SC-003: a tool credential
// may serve on its own service subject but is DENIED publishing a topic op — a
// tool only serves, it does not participate.
func TestToolCredentialScopeEnforced(t *testing.T) {
	op := natstest.StartOperator(t)
	defer op.Shutdown()

	m, err := minter.NewSigningKeyMinter(op.AccountSigningSeed, op.RootAccountKey, []string{op.URL})
	if err != nil {
		t.Fatalf("minter: %v", err)
	}
	cred, err := m.Mint(minter.Scope{Role: declaration.RoleTool, Persona: "uppercase", Topic: "t-ab12"}, time.Hour)
	if err != nil {
		t.Fatalf("mint tool: %v", err)
	}

	permErr := make(chan error, 8)
	nc, err := nats.Connect(op.URL,
		nats.UserCredentials(writeCreds(t, cred)),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, e error) { permErr <- e }),
	)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()

	// In scope: the tool may subscribe its own service subject.
	if _, err := nc.SubscribeSync(minter.ServiceSubject("uppercase")); err != nil {
		t.Fatalf("in-scope subscribe: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Out of scope: a tool must not publish topic ops (it is not a participant).
	if err := nc.Publish(topic.OpsSubject("t-ab12"), []byte("nope")); err != nil {
		t.Fatalf("publish call: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	select {
	case e := <-permErr:
		if !strings.Contains(strings.ToLower(e.Error()), "permission") {
			t.Fatalf("expected a permissions violation, got: %v", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SC-003: a tool publishing topic ops was NOT denied")
	}
}

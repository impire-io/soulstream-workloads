package integration

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/internal/natstest"
	"github.com/impire-io/soulstream-workloads/minter"
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

// TestCapabilityScopeEnforced is spec 010 SC-001/SC-002: a capability
// credential reaches exactly its declared tools. The granted tool answers;
// an ungranted publish dies at the server with zero deliveries to its
// responder; a role-only (zero-tool) credential admits and reaches no tool
// subject. Zero authorization code runs anywhere but the server.
func TestCapabilityScopeEnforced(t *testing.T) {
	op := natstest.StartOperator(t)
	defer op.Shutdown()

	m, err := minter.NewSigningKeyMinter(op.AccountSigningSeed, op.RootAccountKey, []string{op.URL})
	if err != nil {
		t.Fatalf("minter: %v", err)
	}

	// Two tools serve on their own minted tool credentials; toolb counts
	// what it receives so the refusal below can prove zero deliveries.
	serveTool := func(name string, deliveries *int32) *nats.Conn {
		t.Helper()
		cred, err := m.Mint(minter.Scope{Role: declaration.RoleTool, Persona: name, Topic: "t-ab12"}, time.Hour)
		if err != nil {
			t.Fatalf("mint tool %s: %v", name, err)
		}
		nc, err := nats.Connect(op.URL, nats.UserCredentials(writeCreds(t, cred)))
		if err != nil {
			t.Fatalf("connect tool %s: %v", name, err)
		}
		if _, err := nc.Subscribe(minter.ServiceSubject(name), func(msg *nats.Msg) {
			atomic.AddInt32(deliveries, 1)
			_ = msg.Respond([]byte("answered by " + name))
		}); err != nil {
			t.Fatalf("subscribe tool %s: %v", name, err)
		}
		if err := nc.Flush(); err != nil {
			t.Fatalf("flush tool %s: %v", name, err)
		}
		return nc
	}
	var toolaSeen, toolbSeen int32
	nca := serveTool("toola", &toolaSeen)
	defer nca.Close()
	ncb := serveTool("toolb", &toolbSeen)
	defer ncb.Close()

	// The capability agent: granted toola only.
	cred, err := m.Mint(minter.Scope{
		Persona: "sprite", Topic: "t-ab12",
		Capabilities: &declaration.Capabilities{Role: "agent", Tools: []string{"toola"}},
	}, time.Hour)
	if err != nil {
		t.Fatalf("mint capability agent: %v", err)
	}
	permErr := make(chan error, 8)
	nc, err := nats.Connect(op.URL,
		nats.UserCredentials(writeCreds(t, cred)),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, e error) { permErr <- e }),
	)
	if err != nil {
		t.Fatalf("connect capability agent: %v", err)
	}
	defer nc.Close()

	// Granted: the declared tool answers (SC-001, positive arm).
	resp, err := nc.Request(minter.ServiceSubject("toola"), []byte("ping"), 3*time.Second)
	if err != nil {
		t.Fatalf("granted tool call failed: %v", err)
	}
	if string(resp.Data) != "answered by toola" {
		t.Fatalf("granted tool answered %q", resp.Data)
	}

	// Ungranted: the publish must die at the server (SC-001, refusal arm).
	if err := nc.Publish(minter.ServiceSubject("toolb"), []byte("nope")); err != nil {
		t.Fatalf("ungranted publish call: %v", err)
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
		t.Fatal("SC-001: ungranted tool publish was NOT denied by the server")
	}
	if n := atomic.LoadInt32(&toolbSeen); n != 0 {
		t.Fatalf("SC-001: the ungranted responder received %d deliveries, want 0", n)
	}

	// Role-only: admits, reaches nothing on the tool namespace (SC-002).
	roCred, err := m.Mint(minter.Scope{
		Persona: "lone", Topic: "t-ab12",
		Capabilities: &declaration.Capabilities{Role: "agent"},
	}, time.Hour)
	if err != nil {
		t.Fatalf("mint role-only: %v", err)
	}
	roPerm := make(chan error, 8)
	ro, err := nats.Connect(op.URL,
		nats.UserCredentials(writeCreds(t, roCred)),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, e error) { roPerm <- e }),
	)
	if err != nil {
		t.Fatalf("SC-002: role-only credential did not admit: %v", err)
	}
	defer ro.Close()
	if err := ro.Publish(minter.ServiceSubject("toola"), []byte("nope")); err != nil {
		t.Fatalf("role-only publish call: %v", err)
	}
	if err := ro.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	select {
	case e := <-roPerm:
		if !strings.Contains(strings.ToLower(e.Error()), "permission") {
			t.Fatalf("expected a permissions violation, got: %v", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SC-002: a zero-tool credential reached the tool namespace")
	}
}

package waker

import (
	"context"
	"testing"

	"github.com/impire-io/soulstream-core/realm"

	"github.com/impire-io/soulstream-workloads/internal/natstest"
)

func dialConfig(persona, url string) realm.Config {
	return realm.Config{URL: url, Realm: "test-realm", Persona: persona}
}

// A real operator-mode server's admission refusal classifies as the refused
// wake class — the enforcement is the server's, the classification ours. An
// unreachable address classifies as the transient class. (Full token-lane
// revocation → next-wake refusal was measured on the product stack in the
// research; the waker's contract is the class behavior.)
func TestDialClassifiesRealRefusal(t *testing.T) {
	op := natstest.StartOperator(t)
	t.Cleanup(op.Shutdown)

	// No credential at all against an operator-mode server: refused.
	_, err := dialAgent(context.Background(), dialConfig("clerk", op.URL))
	if err == nil {
		t.Fatal("operator server admitted a credential-less dial")
	}
	if !isRefused(err) {
		t.Fatalf("refusal classified as transient: %v", err)
	}

	// A dead address: transient, never refused.
	_, err = dialAgent(context.Background(), dialConfig("clerk", "nats://127.0.0.1:1"))
	if err == nil {
		t.Fatal("dial to a dead address succeeded")
	}
	if isRefused(err) {
		t.Fatalf("transport failure classified as refusal: %v", err)
	}
}

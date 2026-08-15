//go:build wrap_e2e

package integration

import (
	"os/exec"
	"testing"
	"time"

	"github.com/impire-io/soulstream-workloads/wrap"
)

// The real thing: an actual headless claude-code answers a mention through
// the wrapper, exactly as `soulstream wrap --harness claude` would run it.
// Opt-in (`make test-wrap`): needs `claude` installed and logged in on this
// machine; the hermetic suite proves the same protocol with harness-mock.
func TestWrapLiveClaude(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Fatalf("claude not on PATH — this proof needs the operator's assistant: %v", err)
	}
	rig, path := startWrapRealm(t)

	tpl, err := wrap.Preset("claude", wrap.Lane{
		URL: rig.url, Realm: "test-realm", Persona: "clerk",
	})
	if err != nil {
		t.Fatalf("preset: %v", err)
	}
	mention := post(t, rig.owner, path, "Hello @clerk — please confirm you hear us, one sentence.")
	runWrapper(t, rig, tpl, 150*time.Second, 2)

	replies := waitTurns(t, rig.owner, path, "clerk", 1)
	if replies[0].OpID != wrap.WakeOpID(mention, "clerk") {
		t.Fatalf("reply op = %s, want the deterministic wake id", replies[0].OpID)
	}
	t.Logf("live claude replied: %s", replies[0].Body)
}

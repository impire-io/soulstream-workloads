//go:build wake_e2e

package integration

import (
	"os/exec"
	"testing"
	"time"

	"github.com/impire-io/soulstream-workloads/waker"
)

// The M3.2 real-harness proof: an actual headless claude-code run answers a
// mention through the full wake path — the quickstart §3 scenario as a test.
// Opt-in (`make test-wake`): needs `claude` installed and logged in on this
// machine; the hermetic suite proves the same protocol with harness-mock.
func TestWakerLiveClaude(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Fatalf("claude not on PATH — this proof needs the operator's harness: %v", err)
	}
	rig, path := startWakerRealm(t)

	reg := waker.Registration{
		Persona:    "clerk",
		Credential: waker.Credential{URL: rig.url},
		MaxDeliver: 2,
		RunTimeout: waker.Duration(150 * time.Second),
		Template: waker.Template{
			Command: []string{"claude", "-p", "{{PROMPT}}", "--output-format", "stream-json",
				"--verbose", "--model", "haiku", "--max-turns", "4", "--strict-mcp-config"},
			Prompt: "You are @{{PERSONA}}. @{{AUTHOR}} said: {{BODY}}\n\nYour final message is posted as your reply. Reply with one short sentence.",
			Terminal: waker.TerminalMap{TypeField: "type", TerminalValue: "result",
				TextField: "result", StatusField: "subtype", SuccessValue: "success"},
		},
	}
	mention := post(t, rig.owner, path, "Hello @clerk — please confirm you hear us, one sentence.")
	rig.serve(t, rig.config(t, reg))

	replies := waitTurns(t, rig.owner, path, "clerk", 1)
	if replies[0].OpID != waker.WakeOpID(mention, "clerk") {
		t.Fatalf("reply op = %s, want the deterministic wake id", replies[0].OpID)
	}
	t.Logf("live claude replied: %s", replies[0].Body)
	assertConsumerDrained(t, rig.url, "clerk")
}

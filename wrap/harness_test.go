package wrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func claudeMap() TerminalMap {
	return TerminalMap{TypeField: "type", TerminalValue: "result", TextField: "result",
		StatusField: "subtype", SuccessValue: "success"}
}

func codexMap() TerminalMap {
	return TerminalMap{TypeField: "msg.type", TerminalValue: "task_complete",
		TextField: "msg.last_agent_message"}
}

// shTemplate runs an inline shell script as the harness.
func shTemplate(script string, m TerminalMap) Template {
	return Template{
		Command:  []string{"/bin/sh", "-c", script},
		Prompt:   "unused",
		Terminal: m,
	}
}

func run(t *testing.T, tpl Template, timeout time.Duration) HarnessResult {
	t.Helper()
	return RunHarness(context.Background(), RunSpec{
		Template: tpl,
		Prompt:   "p",
		RunDir:   filepath.Join(t.TempDir(), "run"),
		Timeout:  timeout,
	})
}

// Both measured grammars extract through the mapping alone — the same code
// path serves a flat and a nested terminal event (SC-004's unit-level guard).
func TestExtractBothGrammars(t *testing.T) {
	cases := []struct {
		name string
		tpl  Template
		want string
	}{
		{"claude flat", shTemplate(
			`echo '{"type":"system"}'; echo '{"type":"result","subtype":"success","result":"pong"}'`,
			claudeMap()), "pong"},
		{"codex nested", shTemplate(
			`echo '{"id":"1","msg":{"type":"task_started"}}'; echo '{"id":"1","msg":{"type":"task_complete","last_agent_message":"done deal"}}'`,
			codexMap()), "done deal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, tc.tpl, 10*time.Second)
			if !res.OK || res.Text != tc.want {
				t.Fatalf("result = %+v, want OK text %q", res, tc.want)
			}
		})
	}
}

// Failure classes carry a legible reason: error status, missing terminal,
// empty text, death, and timeout are all distinct sentences.
func TestFailureClasses(t *testing.T) {
	cases := []struct {
		name string
		tpl  Template
		want string
	}{
		{"error status", shTemplate(
			`echo '{"type":"result","subtype":"error_max_turns","result":"gave up"}'`,
			claudeMap()), "terminal status"},
		{"no terminal event", shTemplate(`echo '{"type":"system"}'`, claudeMap()), "no terminal event"},
		{"empty text", shTemplate(
			`echo '{"type":"result","subtype":"success","result":"  "}'`,
			claudeMap()), "carried no text"},
		{"harness died", shTemplate(`exit 3`, claudeMap()), "harness died"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := run(t, tc.tpl, 10*time.Second)
			if res.OK || !strings.Contains(res.Detail, tc.want) {
				t.Fatalf("result = %+v, want failure mentioning %q", res, tc.want)
			}
		})
	}
}

// A run past its budget is killed — the whole process group — and says so.
func TestTimeoutKillsProcessGroup(t *testing.T) {
	res := run(t, shTemplate(`sleep 30`, claudeMap()), time.Second)
	if res.OK || !strings.Contains(res.Detail, "run timeout") {
		t.Fatalf("result = %+v, want a run-timeout failure", res)
	}
}

// The child environment carries no SOULSTREAM_* from the host — the person's
// own realm configuration cannot leak into a harness — while the template's
// own env block IS applied: the lane for a per-agent provider credential.
func TestChildEnvironment(t *testing.T) {
	t.Setenv("SOULSTREAM_CONTEXT", "the-persons-own")
	tpl := shTemplate(`echo "{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"realm=$SOULSTREAM_CONTEXT key=$FAKE_PROVIDER_KEY\"}"`, claudeMap())
	tpl.Env = map[string]string{"FAKE_PROVIDER_KEY": "per-agent"}
	res := run(t, tpl, 10*time.Second)
	if !res.OK || res.Text != "realm= key=per-agent" {
		t.Fatalf("result = %+v, want the realm variable scrubbed and the template env applied", res)
	}
}

// The generated MCP config carries the template's tool-door block, 0600 —
// it may hold a credential.
func TestMCPConfigFromTemplate(t *testing.T) {
	dir := t.TempDir()
	spec := RunSpec{
		Template: Template{
			Command:    []string{"/bin/sh", "-c", "true"},
			Prompt:     "p",
			Terminal:   claudeMap(),
			MCPCommand: "soulstream-mcp",
			MCPEnv: map[string]string{
				"SOULSTREAM_TOKEN": "sit_deadbeef",
				"SOULSTREAM_REALM": "acme",
			},
		},
		Prompt:  "p",
		RunDir:  filepath.Join(dir, "run"),
		Timeout: 10 * time.Second,
	}
	_ = RunHarness(context.Background(), spec)
	raw, err := os.ReadFile(filepath.Join(spec.RunDir, "mcp.json"))
	if err != nil {
		t.Fatalf("mcp config: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "sit_deadbeef") || !strings.Contains(s, "acme") || !strings.Contains(s, "soulstream-mcp") {
		t.Fatalf("mcp config missing template values:\n%s", s)
	}
	info, err := os.Stat(filepath.Join(spec.RunDir, "mcp.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mcp config mode = %v, want 0600", info.Mode().Perm())
	}
}

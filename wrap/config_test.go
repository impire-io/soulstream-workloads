package wrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The claude preset derives its MCP environment from the wrapper's own lane
// — the common case authors nothing: the same values that admitted the
// wrapper admit its harness's tool door.
func TestClaudePresetDerivesMCPFromLane(t *testing.T) {
	tpl, err := Preset("claude", Lane{
		URL: "nats://h:4222", CredsFile: "/s/sentinel.creds", Token: "sit_x",
		Realm: "home", Persona: "clerk",
	})
	if err != nil {
		t.Fatalf("preset: %v", err)
	}
	if err := tpl.Validate(); err != nil {
		t.Fatalf("preset template invalid: %v", err)
	}
	if tpl.MCPCommand != "soulstream-mcp" {
		t.Fatalf("mcp command = %q", tpl.MCPCommand)
	}
	for k, want := range map[string]string{
		"SOULSTREAM_URL": "nats://h:4222", "SOULSTREAM_CREDS": "/s/sentinel.creds",
		"SOULSTREAM_TOKEN": "sit_x", "SOULSTREAM_REALM": "home", "SOULSTREAM_PERSONA": "clerk",
	} {
		if tpl.MCPEnv[k] != want {
			t.Fatalf("mcp env %s = %q, want %q", k, tpl.MCPEnv[k], want)
		}
	}
}

// A caller that carries the tool door inside itself points the preset at
// its own executable and names the verb that speaks MCP — the lane's args
// ride into the template unchanged.
func TestClaudePresetCarriesTheSubcommandDoor(t *testing.T) {
	tpl, err := Preset("claude", Lane{
		Persona: "clerk", MCPCommandLoc: "/opt/soulstream", MCPArgs: []string{"mcp"},
	})
	if err != nil {
		t.Fatalf("preset: %v", err)
	}
	if tpl.MCPCommand != "/opt/soulstream" || len(tpl.MCPArgs) != 1 || tpl.MCPArgs[0] != "mcp" {
		t.Fatalf("door = %q %v, want /opt/soulstream [mcp]", tpl.MCPCommand, tpl.MCPArgs)
	}
}

// The codex preset owns the envelope alone (its MCP config is its own global
// file); an unknown name is refused with the two that exist.
func TestCodexPresetAndUnknown(t *testing.T) {
	tpl, err := Preset("codex", Lane{Persona: "clerk"})
	if err != nil || tpl.MCPCommand != "" {
		t.Fatalf("codex preset = %+v, %v", tpl, err)
	}
	if tpl.Terminal.TypeField != "msg.type" {
		t.Fatalf("codex terminal = %+v", tpl.Terminal)
	}
	if _, err := Preset("mystery", Lane{}); err == nil || !strings.Contains(err.Error(), "claude, codex") {
		t.Fatalf("unknown preset err = %v", err)
	}
}

// Template files keep the specs/005 refusal rules: strict decode, required
// terminal mapping, paired status fields.
func TestLoadTemplateRefusals(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "tpl.json")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cases := []struct {
		name string
		body string
		want string
	}{
		{"unknown field", `{"command":["x"],"prompt":"p","surprise":1,
			"terminal":{"type_field":"t","terminal_value":"v","text_field":"x"}}`, "unknown field"},
		{"no terminal", `{"command":["x"],"prompt":"p","terminal":{"type_field":"t"}}`,
			"machine-readable terminal event"},
		{"half status", `{"command":["x"],"prompt":"p",
			"terminal":{"type_field":"t","terminal_value":"v","text_field":"x","status_field":"s"}}`,
			"come together"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadTemplate(write(t, tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
	good := write(t, `{"command":["x","{{PROMPT}}"],"prompt":"p","env":{"K":"v"},
		"mcp_command":"soulstream","mcp_args":["mcp"],
		"terminal":{"type_field":"t","terminal_value":"v","text_field":"x"}}`)
	tpl, err := LoadTemplate(good)
	if err != nil || tpl.Env["K"] != "v" {
		t.Fatalf("good template = %+v, %v", tpl, err)
	}
	if len(tpl.MCPArgs) != 1 || tpl.MCPArgs[0] != "mcp" {
		t.Fatalf("mcp_args = %v, want [mcp]", tpl.MCPArgs)
	}
}

// The declared extras ride into the door's environment beside the lane —
// and the lane's own keys stay the lane's.
func TestPresetCarriesDeclaredExtraEnv(t *testing.T) {
	tpl, err := Preset("claude", Lane{
		URL: "nats://127.0.0.1:4222", Token: "sit_x", Realm: "acme", Persona: "scribe",
		MCPExtraEnv: map[string]string{
			"SOULSTREAM_SUBJECT": "daan",
			"SOULSTREAM_TOKEN":   "overridden-never",
			"EMPTY":              "",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := tpl.MCPEnv["SOULSTREAM_SUBJECT"]; got != "daan" {
		t.Fatalf("extra env did not ride: %q", got)
	}
	if got := tpl.MCPEnv["SOULSTREAM_TOKEN"]; got != "sit_x" {
		t.Fatalf("an extra overrode the lane's own key: %q", got)
	}
	if _, ok := tpl.MCPEnv["EMPTY"]; ok {
		t.Fatal("an empty extra was carried")
	}
}

// An untouched budget gets the design defaults (hq design 0006 §3); the
// opt-out is the explicit Unbudgeted field, never a clever zero reading.
func TestBudgetDefaults(t *testing.T) {
	cfg := Config{Persona: "clerk"}
	cfg.ApplyDefaults()
	want := Budget{MaxHops: 4, WindowMax: 8, WindowPer: 10 * time.Minute}
	if cfg.Budget != want {
		t.Fatalf("default budget = %+v, want %+v", cfg.Budget, want)
	}

	out := Config{Persona: "clerk", Unbudgeted: true}
	out.ApplyDefaults()
	if out.Budget != (Budget{}) {
		t.Fatalf("unbudgeted config grew a budget: %+v", out.Budget)
	}
}

// A partly-set budget is taken as written: a zero part is that part
// disabled, not an invitation to fill it.
func TestBudgetPartialIsTakenAsWritten(t *testing.T) {
	cfg := Config{Persona: "clerk", Budget: Budget{WindowMax: 20, WindowPer: time.Hour}}
	cfg.ApplyDefaults()
	if cfg.Budget.MaxHops != 0 || cfg.Budget.WindowMax != 20 || cfg.Budget.WindowPer != time.Hour {
		t.Fatalf("partial budget rewritten: %+v", cfg.Budget)
	}
}

// A budget that cannot mean anything is refused with a teaching error.
func TestBudgetValidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		b    Budget
		ok   bool
	}{
		{"zero is fine", Budget{}, true},
		{"defaults are fine", Budget{MaxHops: 4, WindowMax: 8, WindowPer: 10 * time.Minute}, true},
		{"depth only", Budget{MaxHops: 2}, true},
		{"negative hops", Budget{MaxHops: -1}, false},
		{"negative window", Budget{WindowMax: -1, WindowPer: time.Minute}, false},
		{"negative duration", Budget{WindowMax: 1, WindowPer: -time.Minute}, false},
		{"count without duration", Budget{WindowMax: 3}, false},
	} {
		err := tc.b.Validate()
		if (err == nil) != tc.ok {
			t.Errorf("%s: Validate() = %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
}

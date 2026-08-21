package wrap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config is one wrapped agent: the persona (the wrapper's whole authority is
// that persona's own credential, resolved by the caller into the realm
// client), the harness template, and the budgets.
type Config struct {
	Persona    string
	Template   Template
	Scratch    string
	RunTimeout time.Duration
	Retries    int // harness attempts per wake before the self-report
	InboxLimit int // catch-up depth (0 = the protocol default of 50)
}

// Template is one harness as configuration — the whole difference between
// claude, codex, and a shell script. Env is the harness's own environment
// (a provider key like ANTHROPIC_API_KEY when the host's login state is not
// the lane); MCPCommand/MCPArgs/MCPEnv describe the tool door written per
// run. Args exist so a subcommand can be the door — a caller that carries
// the door inside itself points command at its own executable and says
// which verb speaks MCP.
type Template struct {
	Command    []string          `json:"command"`
	Prompt     string            `json:"prompt"`
	Terminal   TerminalMap       `json:"terminal"`
	Env        map[string]string `json:"env,omitempty"`
	MCPCommand string            `json:"mcp_command,omitempty"`
	MCPArgs    []string          `json:"mcp_args,omitempty"`
	MCPEnv     map[string]string `json:"mcp_env,omitempty"`
}

// TerminalMap locates the terminal event in the harness's JSONL output.
// Fields are dot-paths ("type", "msg.type"). A template without one is
// refused: a harness that cannot say, in a typed way, that it is done and
// what it concluded cannot be wrapped.
type TerminalMap struct {
	TypeField     string `json:"type_field"`
	TerminalValue string `json:"terminal_value"`
	TextField     string `json:"text_field"`
	StatusField   string `json:"status_field,omitempty"`
	SuccessValue  string `json:"success_value,omitempty"`
}

const (
	defaultRetries    = 2
	defaultRunTimeout = 150 * time.Second
)

// Validate refuses what specs/005 refused: no command, no prompt, no
// machine-readable terminal event, or a half-declared status pair.
func (t *Template) Validate() error {
	if len(t.Command) == 0 {
		return fmt.Errorf("wrap: template.command is required")
	}
	if t.Prompt == "" {
		return fmt.Errorf("wrap: template.prompt is required")
	}
	m := t.Terminal
	if m.TypeField == "" || m.TerminalValue == "" || m.TextField == "" {
		return fmt.Errorf("wrap: template.terminal needs type_field, terminal_value and text_field — a harness without a machine-readable terminal event cannot be wrapped")
	}
	if (m.StatusField == "") != (m.SuccessValue == "") {
		return fmt.Errorf("wrap: template.terminal status_field and success_value come together or not at all")
	}
	return nil
}

// ApplyDefaults fills the budgets people rarely set.
func (c *Config) ApplyDefaults() {
	if c.Retries == 0 {
		c.Retries = defaultRetries
	}
	if c.RunTimeout == 0 {
		c.RunTimeout = defaultRunTimeout
	}
}

// LoadTemplate reads a custom template file, strict-decoded: an unknown
// field is a refusal, never a silent ignore.
func LoadTemplate(path string) (Template, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Template{}, fmt.Errorf("wrap: read template: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var t Template
	if err := dec.Decode(&t); err != nil {
		return Template{}, fmt.Errorf("wrap: parse template: %w", err)
	}
	if dec.More() {
		return Template{}, fmt.Errorf("wrap: template has trailing content after the object")
	}
	if err := t.Validate(); err != nil {
		return Template{}, err
	}
	return t, nil
}

// Lane is what the wrapper knows about its own connection, reused to derive
// a preset's MCP environment — the common case authors nothing: the same
// five values that admitted the wrapper admit its harness's tool door.
type Lane struct {
	URL           string
	CredsFile     string
	Token         string
	Realm         string
	Persona       string
	MCPCommandLoc string   // "" = "soulstream-mcp" from PATH
	MCPArgs       []string // args before the door speaks MCP — a subcommand door
	// MCPExtraEnv rides into the door's environment beside the lane —
	// the stated surface for a door's outbound identity (the subject a
	// personal agent acts for and the delegation authorizing it, hq
	// design external-tools.md D41). Deliberately explicit: wrap scrubs
	// the host's SOULSTREAM_* from the harness on purpose, so anything a
	// door needs arrives by declaration, never by inheritance.
	MCPExtraEnv map[string]string
}

// Preset returns a named built-in template. The two names are the two
// measured harnesses; anything else wants a template file.
func Preset(name string, lane Lane) (Template, error) {
	mcpCommand := lane.MCPCommandLoc
	if mcpCommand == "" {
		mcpCommand = "soulstream-mcp"
	}
	mcpEnv := map[string]string{}
	for k, v := range map[string]string{
		"SOULSTREAM_URL":     lane.URL,
		"SOULSTREAM_CREDS":   lane.CredsFile,
		"SOULSTREAM_TOKEN":   lane.Token,
		"SOULSTREAM_REALM":   lane.Realm,
		"SOULSTREAM_PERSONA": lane.Persona,
	} {
		if v != "" {
			mcpEnv[k] = v
		}
	}
	// The declared extras ride last: the lane's own keys stay the lane's.
	for k, v := range lane.MCPExtraEnv {
		if _, taken := mcpEnv[k]; !taken && v != "" {
			mcpEnv[k] = v
		}
	}
	switch name {
	case "claude":
		return Template{
			Command: []string{"claude", "-p", "{{PROMPT}}", "--output-format", "stream-json",
				"--verbose", "--mcp-config", "{{MCP_CONFIG}}", "--strict-mcp-config",
				"--allowedTools", "mcp__soulstream__*", "--max-turns", "8"},
			Prompt: presetPrompt,
			Terminal: TerminalMap{TypeField: "type", TerminalValue: "result",
				TextField: "result", StatusField: "subtype", SuccessValue: "success"},
			MCPCommand: mcpCommand,
			MCPArgs:    lane.MCPArgs,
			MCPEnv:     mcpEnv,
		}, nil
	case "codex":
		// Codex reads its MCP servers from its own global config (the shell's
		// set-up fold covers it); the preset owns the envelope alone.
		return Template{
			Command: []string{"codex", "exec", "--json", "--skip-git-repo-check", "{{PROMPT}}"},
			Prompt:  presetPrompt,
			Terminal: TerminalMap{TypeField: "msg.type", TerminalValue: "task_complete",
				TextField: "msg.last_agent_message"},
		}, nil
	default:
		return Template{}, fmt.Errorf("wrap: no preset %q (claude, codex) — pass a template file for anything else", name)
	}
}

const presetPrompt = "You are @{{PERSONA}}, an agent in a shared conversation space. " +
	"@{{AUTHOR}} mentioned you in the topic \"{{TOPIC}}\" (message {{OP_ID}}):\n\n{{BODY}}\n\n" +
	"If tools named soulstream_* are available, you may read the topic for context " +
	"(soulstream_show_topic with path \"{{TOPIC}}\") and act on what was asked. " +
	"Do NOT post your main reply with a tool — your final message is posted into the " +
	"topic as your reply automatically. Write it as the reply itself, addressed to the conversation."

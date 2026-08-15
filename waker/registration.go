package waker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config is the waker's whole operator surface: who the waker itself is, and
// which agents it serves. Strict-decoded like a declaration — an unknown field
// is a refusal, never a silent ignore.
type Config struct {
	Waker  Identity       `json:"waker"`
	Agents []Registration `json:"agents"`
}

// Identity is the waker's own standing: the NATS context carrying its
// operator-provisioned credential, the realm, the persona its failure
// testimony is authored by, and the scratch root for run directories.
// IdentityPlane is required exactly when a registration declares the
// ephemeral lane — minting rides the waker's own connection.
type Identity struct {
	Context       string         `json:"context"`
	Realm         string         `json:"realm"`
	Persona       string         `json:"persona"`
	Scratch       string         `json:"scratch"`
	IdentityPlane *IdentityPlane `json:"identity_plane,omitempty"`
}

// IdentityPlane names the waker's principal on the identity plane's sealed
// surface: the realm account and the user the waker's credential carries.
type IdentityPlane struct {
	Account string `json:"account"`
	User    string `json:"user"`
}

// Registration is one agent as the waker serves it: the persona whose notify
// subject is consumed, the credential lane its wakes ride, and the invocation
// template that runs one turn of its harness.
type Registration struct {
	Persona    string     `json:"persona"`
	Credential Credential `json:"credential"`
	MaxDeliver int        `json:"max_deliver,omitempty"`
	RunTimeout Duration   `json:"run_timeout,omitempty"`
	Template   Template   `json:"template"`
}

// Credential is the wake lane. URL is always required — it is where the agent
// dials. The lanes: sentinel+token (the revocable registration; the dial is
// the admission probe), ephemeral (the waker mints a run-bounded credential
// per wake), or neither (direct dial — open rigs, where admission is not the
// thing under test). Sentinel/token and ephemeral are mutually exclusive.
type Credential struct {
	URL           string         `json:"url"`
	SentinelCreds string         `json:"sentinel_creds,omitempty"`
	Token         string         `json:"token,omitempty"`
	Ephemeral     *EphemeralLane `json:"ephemeral,omitempty"`
}

// EphemeralLane asks the waker to mint a per-run credential against a declared
// role, with a lifetime bounding the run.
type EphemeralLane struct {
	Role string   `json:"role"`
	TTL  Duration `json:"ttl"`
}

// Template is one harness as configuration: how to run one turn of it and how
// to read its typed terminal event. The waker holds no harness-specific code —
// this struct is the whole difference between claude, codex, and a shell
// script.
type Template struct {
	Command    []string          `json:"command"`
	Prompt     string            `json:"prompt"`
	Terminal   TerminalMap       `json:"terminal"`
	MCPCommand string            `json:"mcp_command,omitempty"`
	MCPEnv     map[string]string `json:"mcp_env,omitempty"`
}

// TerminalMap locates the terminal event in the harness's JSONL output.
// Fields are dot-paths ("type", "msg.type"). A template without one is
// refused at load: a harness that cannot say, in a typed way, that it is done
// and what it concluded cannot be registered.
type TerminalMap struct {
	TypeField     string `json:"type_field"`
	TerminalValue string `json:"terminal_value"`
	TextField     string `json:"text_field"`
	StatusField   string `json:"status_field,omitempty"`
	SuccessValue  string `json:"success_value,omitempty"`
}

// Duration is a JSON-friendly time.Duration ("150s", "2m").
type Duration time.Duration

// UnmarshalJSON parses Go duration syntax.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("waker: duration must be a string like \"150s\": %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("waker: parse duration %q: %w", s, err)
	}
	*d = Duration(v)
	return nil
}

// MarshalJSON renders Go duration syntax.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

const (
	defaultMaxDeliver = 2
	defaultRunTimeout = 150 * time.Second
)

// Load reads and validates a waker configuration file, applying defaults.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("waker: read config: %w", err)
	}
	return Parse(raw)
}

// Parse strict-decodes and validates a waker configuration, applying defaults.
func Parse(raw []byte) (Config, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("waker: parse config: %w", err)
	}
	if dec.More() {
		return Config{}, fmt.Errorf("waker: config has trailing content after the object")
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Waker.Context == "" {
		return fmt.Errorf("waker: waker.context is required (the NATS context carrying the waker's own credential)")
	}
	if c.Waker.Realm == "" {
		return fmt.Errorf("waker: waker.realm is required")
	}
	if c.Waker.Persona == "" {
		return fmt.Errorf("waker: waker.persona is required (failure testimony needs an author)")
	}
	if c.Waker.Scratch == "" {
		return fmt.Errorf("waker: waker.scratch is required (run directories need a root)")
	}
	if len(c.Agents) == 0 {
		return fmt.Errorf("waker: at least one agent registration is required")
	}
	seen := make(map[string]bool, len(c.Agents))
	ephemeral := false
	for i := range c.Agents {
		if err := c.Agents[i].validate(); err != nil {
			return err
		}
		if seen[c.Agents[i].Persona] {
			return fmt.Errorf("waker: agent %q is registered twice", c.Agents[i].Persona)
		}
		seen[c.Agents[i].Persona] = true
		ephemeral = ephemeral || c.Agents[i].EphemeralLane()
	}
	if ephemeral && c.Waker.IdentityPlane == nil {
		return fmt.Errorf("waker: an ephemeral-lane registration needs waker.identity_plane (account and user for the mint)")
	}
	if p := c.Waker.IdentityPlane; p != nil && (p.Account == "" || p.User == "") {
		return fmt.Errorf("waker: identity_plane needs both account and user")
	}
	return nil
}

func (r *Registration) validate() error {
	if r.Persona == "" {
		return fmt.Errorf("waker: a registration needs a persona")
	}
	if r.MaxDeliver < 0 {
		return fmt.Errorf("waker: agent %q: max_deliver must be positive", r.Persona)
	}
	if err := r.Credential.validate(r.Persona); err != nil {
		return err
	}
	return r.Template.validate(r.Persona)
}

func (c *Credential) validate(persona string) error {
	if c.URL == "" {
		return fmt.Errorf("waker: agent %q: credential.url is required (where the agent dials)", persona)
	}
	tokenLane := c.SentinelCreds != "" || c.Token != ""
	if tokenLane && (c.SentinelCreds == "" || c.Token == "") {
		return fmt.Errorf("waker: agent %q: the token lane needs both sentinel_creds and token", persona)
	}
	if tokenLane && c.Ephemeral != nil {
		return fmt.Errorf("waker: agent %q: sentinel/token and ephemeral are mutually exclusive lanes", persona)
	}
	if c.Ephemeral != nil {
		if c.Ephemeral.Role == "" {
			return fmt.Errorf("waker: agent %q: ephemeral.role is required", persona)
		}
		if c.Ephemeral.TTL <= 0 {
			return fmt.Errorf("waker: agent %q: ephemeral.ttl must be positive", persona)
		}
	}
	return nil
}

func (t *Template) validate(persona string) error {
	if len(t.Command) == 0 {
		return fmt.Errorf("waker: agent %q: template.command is required", persona)
	}
	if t.Prompt == "" {
		return fmt.Errorf("waker: agent %q: template.prompt is required", persona)
	}
	m := t.Terminal
	if m.TypeField == "" || m.TerminalValue == "" || m.TextField == "" {
		return fmt.Errorf("waker: agent %q: template.terminal needs type_field, terminal_value and text_field — a harness without a machine-readable terminal event cannot be registered", persona)
	}
	if (m.StatusField == "") != (m.SuccessValue == "") {
		return fmt.Errorf("waker: agent %q: template.terminal status_field and success_value come together or not at all", persona)
	}
	return nil
}

func (c *Config) applyDefaults() {
	for i := range c.Agents {
		if c.Agents[i].MaxDeliver == 0 {
			c.Agents[i].MaxDeliver = defaultMaxDeliver
		}
		if c.Agents[i].RunTimeout == 0 {
			c.Agents[i].RunTimeout = Duration(defaultRunTimeout)
		}
	}
}

// EphemeralLane reports whether this registration rides the per-run mint lane.
func (r *Registration) EphemeralLane() bool { return r.Credential.Ephemeral != nil }

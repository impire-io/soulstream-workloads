package waker

import (
	"strings"
	"testing"
	"time"
)

func validConfig() string {
	return `{
	  "waker": {"context": "ops", "realm": "acme", "persona": "waker", "scratch": "/tmp/wakes"},
	  "agents": [{
	    "persona": "clerk",
	    "credential": {"url": "nats://127.0.0.1:4222"},
	    "template": {
	      "command": ["mock", "{{PROMPT}}"],
	      "prompt": "reply to @{{AUTHOR}}: {{BODY}}",
	      "terminal": {"type_field": "type", "terminal_value": "result", "text_field": "result"}
	    }
	  }]
	}`
}

// A valid minimal config loads with the documented defaults applied.
func TestParseAppliesDefaults(t *testing.T) {
	cfg, err := Parse([]byte(validConfig()))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	a := cfg.Agents[0]
	if a.MaxDeliver != 2 {
		t.Errorf("max_deliver = %d, want default 2", a.MaxDeliver)
	}
	if time.Duration(a.RunTimeout) != 150*time.Second {
		t.Errorf("run_timeout = %v, want default 150s", time.Duration(a.RunTimeout))
	}
}

// FR-004: a template without a terminal mapping is refused at load; the other
// refusals keep the contract's rules honest one guard at a time.
func TestParseRefusals(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(s string) string
		want   string
	}{
		{"unknown field", func(s string) string {
			return strings.Replace(s, `"agents"`, `"surprise": 1, "agents"`, 1)
		}, "unknown field"},
		{"no terminal mapping", func(s string) string {
			return strings.Replace(s, `"terminal": {"type_field": "type", "terminal_value": "result", "text_field": "result"}`,
				`"terminal": {"type_field": "type"}`, 1)
		}, "machine-readable terminal event"},
		{"half a token lane", func(s string) string {
			return strings.Replace(s, `{"url": "nats://127.0.0.1:4222"}`,
				`{"url": "nats://127.0.0.1:4222", "token": "sit_x"}`, 1)
		}, "both sentinel_creds and token"},
		{"both lanes", func(s string) string {
			return strings.Replace(s, `{"url": "nats://127.0.0.1:4222"}`,
				`{"url": "nats://127.0.0.1:4222", "sentinel_creds": "/s", "token": "sit_x", "ephemeral": {"role": "realm", "ttl": "60s"}}`, 1)
		}, "mutually exclusive"},
		{"no url", func(s string) string {
			return strings.Replace(s, `{"url": "nats://127.0.0.1:4222"}`, `{}`, 1)
		}, "credential.url is required"},
		{"status without success", func(s string) string {
			return strings.Replace(s, `"text_field": "result"`,
				`"text_field": "result", "status_field": "subtype"`, 1)
		}, "come together"},
		{"no agents", func(s string) string {
			return strings.Replace(s, s[strings.Index(s, `"agents"`):], `"agents": []}`, 1)
		}, "at least one agent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.mutate(validConfig())))
			if err == nil {
				t.Fatalf("parse accepted, want refusal mentioning %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("refusal = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// The ephemeral lane parses and reports itself; it requires the waker's
// identity-plane principal, because the waker is the one who mints.
func TestParseEphemeralLane(t *testing.T) {
	s := strings.Replace(validConfig(), `{"url": "nats://127.0.0.1:4222"}`,
		`{"url": "nats://127.0.0.1:4222", "ephemeral": {"role": "realm", "ttl": "60s"}}`, 1)
	if _, err := Parse([]byte(s)); err == nil || !strings.Contains(err.Error(), "identity_plane") {
		t.Fatalf("ephemeral lane without identity_plane: err = %v, want a refusal naming identity_plane", err)
	}
	s = strings.Replace(s, `"scratch": "/tmp/wakes"`,
		`"scratch": "/tmp/wakes", "identity_plane": {"account": "AAAA", "user": "ops"}`, 1)
	cfg, err := Parse([]byte(s))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Agents[0].EphemeralLane() {
		t.Fatal("EphemeralLane() = false, want true")
	}
	if got := time.Duration(cfg.Agents[0].Credential.Ephemeral.TTL); got != time.Minute {
		t.Fatalf("ephemeral ttl = %v, want 1m", got)
	}
}

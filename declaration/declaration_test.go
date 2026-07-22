package declaration

import (
	"strings"
	"testing"
)

func validJSON() string {
	return `{
		"role": "agent",
		"lifecycle": "service",
		"persona": "researcher",
		"topic": "acme-team.q2-planning-ab12",
		"artifact": "file:///opt/agents/researcher"
	}`
}

func TestParseValid(t *testing.T) {
	d, err := Parse([]byte(validJSON()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if d.Role != RoleAgent || d.Lifecycle != LifecycleService {
		t.Fatalf("unexpected role/lifecycle: %q/%q", d.Role, d.Lifecycle)
	}
	if d.Persona != "researcher" || d.Topic != "acme-team.q2-planning-ab12" {
		t.Fatalf("unexpected persona/topic: %q/%q", d.Persona, d.Topic)
	}
	p, err := d.ArtifactPath()
	if err != nil {
		t.Fatalf("ArtifactPath: %v", err)
	}
	if p != "/opt/agents/researcher" {
		t.Fatalf("ArtifactPath = %q", p)
	}
}

func TestValidateAcceptsTool(t *testing.T) {
	d, err := Parse([]byte(validJSON()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	d.Role = RoleTool
	if err := d.Validate(); err != nil {
		t.Fatalf("tool/service should validate: %v", err)
	}
}

// SC-005 / FR-001: a backend-specific key must be rejected at parse time.
func TestParseRejectsBackendField(t *testing.T) {
	withBackend := `{
		"role": "agent",
		"lifecycle": "service",
		"persona": "researcher",
		"topic": "acme-team.q2-planning-ab12",
		"artifact": "file:///opt/agents/researcher",
		"backend": "docker"
	}`
	if _, err := Parse([]byte(withBackend)); err == nil {
		t.Fatal("expected a backend key to be rejected, got nil error")
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	if _, err := Parse([]byte(`{"role":"agent","surprise":1}`)); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}

func TestParseRejectsTrailingContent(t *testing.T) {
	if _, err := Parse([]byte(validJSON() + `{"extra":true}`)); err == nil {
		t.Fatal("expected trailing content to be rejected")
	}
}

func TestValidateSubsetAndFields(t *testing.T) {
	base, err := Parse([]byte(validJSON()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Declaration)
		errHas string
	}{
		{"unknown role", func(d *Declaration) { d.Role = "wizard" }, "not a known role"},
		{"function deferred", func(d *Declaration) { d.Lifecycle = LifecycleFunction }, "not supported yet"},
		{"job deferred", func(d *Declaration) { d.Lifecycle = LifecycleJob }, "not supported yet"},
		{"unknown lifecycle", func(d *Declaration) { d.Lifecycle = "eternal" }, "not a known lifecycle"},
		{"bad persona", func(d *Declaration) { d.Persona = "Researcher" }, "valid persona"},
		{"empty persona", func(d *Declaration) { d.Persona = "" }, "valid persona"},
		{"empty topic", func(d *Declaration) { d.Topic = "" }, "topic is required"},
		{"bad topic segment", func(d *Declaration) { d.Topic = "acme.Q2" }, "invalid segment"},
		{"non-file artifact", func(d *Declaration) { d.Artifact = "nats://obj/thing" }, "not supported yet"},
		{"empty artifact", func(d *Declaration) { d.Artifact = "" }, "artifact is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := base
			tc.mutate(&d)
			err := d.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errHas)
			}
			if !strings.Contains(err.Error(), tc.errHas) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.errHas)
			}
		})
	}
}

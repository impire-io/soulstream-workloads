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
		{"non-file artifact", func(d *Declaration) { d.Artifact = "nats://obj/thing" }, "not supported"},
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

// --- 009: the agent declaration grows wake, instructions, capabilities ---

func validAgentJSON() string {
	return `{
		"role": "agent",
		"lifecycle": "service",
		"persona": "researcher",
		"topic": "acme-team.q2-planning-ab12",
		"artifact": "soulstream://acme-team.q2-planning-ab12/researcher-bin",
		"instructions": {"topic": "acme-team.souls-ab12", "artefact": "researcher.md"},
		"capabilities": {"role": "agent-default", "tools": ["web-fetch", "calc"]},
		"wake": [
			{"kind": "mention"},
			{"kind": "topic", "path": "acme-team.q2-planning-ab12", "types": ["turn.post", "attachment.add"]},
			{"kind": "schedule", "name": "daily", "pattern": "@every 24h", "ttl": "1h"},
			{"kind": "subject", "subject": "acme.events.>"}
		],
		"budget": {"max_hops": 4, "window": {"max": 8, "per": "10m"}}
	}`
}

// FR-001: the full 0005 §2 surface parses and validates.
func TestParseValidAgentDeclaration(t *testing.T) {
	d, err := Parse([]byte(validAgentJSON()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if d.Instructions.Topic != "acme-team.souls-ab12" || d.Instructions.Artefact != "researcher.md" {
		t.Fatalf("instructions = %+v", d.Instructions)
	}
	if d.Capabilities.Role != "agent-default" || len(d.Capabilities.Tools) != 2 {
		t.Fatalf("capabilities = %+v", d.Capabilities)
	}
	if len(d.Wake) != 4 {
		t.Fatalf("wake entries = %d, want 4", len(d.Wake))
	}
	if d.Budget.MaxHops != 4 || d.Budget.Window.Max != 8 || d.Budget.Window.Per != "10m" {
		t.Fatalf("budget = %+v", d.Budget)
	}
	ref, err := d.ArtifactRef()
	if err != nil {
		t.Fatalf("ArtifactRef: %v", err)
	}
	if ref.Scheme != SchemeSoulstream || ref.Topic != "acme-team.q2-planning-ab12" || ref.Name != "researcher-bin" {
		t.Fatalf("ArtifactRef = %+v", ref)
	}
}

// FR-002: the record form validates; malformed record forms refuse.
func TestArtifactRecordForm(t *testing.T) {
	base, _ := Parse([]byte(validJSON()))
	good := base
	good.Artifact = "soulstream://acme-team.q2-planning-ab12/tool.bin"
	if err := good.Validate(); err != nil {
		t.Fatalf("record-form artifact should validate: %v", err)
	}
	ref, err := good.ArtifactRef()
	if err != nil || ref.Scheme != SchemeSoulstream || ref.Name != "tool.bin" {
		t.Fatalf("ArtifactRef = %+v, %v", ref, err)
	}
	if _, err := good.ArtifactPath(); err == nil {
		t.Fatal("ArtifactPath on a record-form artifact must refuse (it is not a host path)")
	}

	fileRef, err := base.ArtifactRef()
	if err != nil || fileRef.Scheme != SchemeFile || fileRef.Path != "/opt/agents/researcher" {
		t.Fatalf("file ArtifactRef = %+v, %v", fileRef, err)
	}

	for name, artifact := range map[string]string{
		"no name":        "soulstream://acme-team.q2-planning-ab12",
		"empty name":     "soulstream://acme-team.q2-planning-ab12/",
		"slash in name":  "soulstream://acme-team.q2-planning-ab12/dir/name",
		"bad topic path": "soulstream://Acme.Q2/name",
		"empty":          "soulstream://",
	} {
		t.Run(name, func(t *testing.T) {
			d := base
			d.Artifact = artifact
			if err := d.Validate(); err == nil {
				t.Fatalf("artifact %q should refuse", artifact)
			}
		})
	}
}

// FR-001: the new blocks are agent-only.
func TestAgentBlocksRefusedForTools(t *testing.T) {
	base, _ := Parse([]byte(validJSON()))
	base.Role = RoleTool
	for name, mutate := range map[string]func(*Declaration){
		"wake":         func(d *Declaration) { d.Wake = []WakeEntry{{Kind: WakeMention}} },
		"instructions": func(d *Declaration) { d.Instructions = &Instructions{Topic: "a-b12", Artefact: "x"} },
		"capabilities": func(d *Declaration) { d.Capabilities = &Capabilities{Role: "r"} },
		"budget":       func(d *Declaration) { d.Budget = &BudgetSpec{MaxHops: 1} },
	} {
		t.Run(name, func(t *testing.T) {
			d := base
			mutate(&d)
			err := d.Validate()
			if err == nil || !strings.Contains(err.Error(), "agent-only") {
				t.Fatalf("err = %v, want the agent-only refusal", err)
			}
		})
	}
}

// FR-003: per-kind field validation and duplicate refusals.
func TestWakeEntryValidation(t *testing.T) {
	base, _ := Parse([]byte(validJSON()))
	cases := []struct {
		name   string
		wake   []WakeEntry
		errHas string
	}{
		{"mention with field", []WakeEntry{{Kind: WakeMention, Path: "x-y12"}}, "does not take path"},
		{"mention with types", []WakeEntry{{Kind: WakeMention, Types: []string{"turn.post"}}}, "does not take types"},
		{"topic without path", []WakeEntry{{Kind: WakeTopic}}, "topic is required"},
		{"topic bad path", []WakeEntry{{Kind: WakeTopic, Path: "Bad.Path"}}, "invalid segment"},
		{"topic bad type", []WakeEntry{{Kind: WakeTopic, Path: "a-b12", Types: []string{"Turn.Post"}}}, "op type"},
		{"topic with subject", []WakeEntry{{Kind: WakeTopic, Path: "a-b12", Subject: "x"}}, "does not take subject"},
		{"schedule without name", []WakeEntry{{Kind: WakeSchedule, Pattern: "@every 1s"}}, "not a valid name"},
		{"schedule bad name", []WakeEntry{{Kind: WakeSchedule, Name: "Daily", Pattern: "@every 1s"}}, "not a valid name"},
		{"schedule without pattern", []WakeEntry{{Kind: WakeSchedule, Name: "daily"}}, "pattern is required"},
		{"schedule bad every", []WakeEntry{{Kind: WakeSchedule, Name: "daily", Pattern: "@every soon"}}, "not a Go duration"},
		{"schedule negative every", []WakeEntry{{Kind: WakeSchedule, Name: "daily", Pattern: "@every -1s"}}, "must be positive"},
		{"schedule bad at", []WakeEntry{{Kind: WakeSchedule, Name: "once", Pattern: "@at tomorrow"}}, "RFC3339"},
		{"schedule unknown at-form", []WakeEntry{{Kind: WakeSchedule, Name: "d", Pattern: "@hourly"}}, "unknown @-form"},
		{"schedule short cron", []WakeEntry{{Kind: WakeSchedule, Name: "d", Pattern: "* * * * *"}}, "6 fields"},
		{"schedule bad cron char", []WakeEntry{{Kind: WakeSchedule, Name: "d", Pattern: "0 0 12 * * ?"}}, "cron field"},
		{"schedule bad ttl", []WakeEntry{{Kind: WakeSchedule, Name: "d", Pattern: "@every 1s", TTL: "long"}}, "not a Go duration"},
		{"schedule zero ttl", []WakeEntry{{Kind: WakeSchedule, Name: "d", Pattern: "@every 1s", TTL: "0s"}}, "must be positive"},
		{"schedule with path", []WakeEntry{{Kind: WakeSchedule, Name: "d", Pattern: "@every 1s", Path: "a-b12"}}, "does not take path"},
		{"subject empty", []WakeEntry{{Kind: WakeSubject}}, "subject is required"},
		{"subject empty token", []WakeEntry{{Kind: WakeSubject, Subject: "a..b"}}, "empty token"},
		{"subject mid gt", []WakeEntry{{Kind: WakeSubject, Subject: "a.>.b"}}, "before the last token"},
		{"subject with pattern", []WakeEntry{{Kind: WakeSubject, Subject: "a.b", Pattern: "@every 1s"}}, "does not take pattern"},
		{"unknown kind", []WakeEntry{{Kind: "cron"}}, "not a known wake kind"},
		{"second mention", []WakeEntry{{Kind: WakeMention}, {Kind: WakeMention}}, "second mention"},
		{"duplicate topic path", []WakeEntry{{Kind: WakeTopic, Path: "a-b12"}, {Kind: WakeTopic, Path: "a-b12"}}, "declared twice"},
		{"duplicate schedule name", []WakeEntry{
			{Kind: WakeSchedule, Name: "d", Pattern: "@every 1s"},
			{Kind: WakeSchedule, Name: "d", Pattern: "@every 2s"}}, "declared twice"},
		{"duplicate subject", []WakeEntry{{Kind: WakeSubject, Subject: "a.b"}, {Kind: WakeSubject, Subject: "a.b"}}, "declared twice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := base
			d.Wake = tc.wake
			err := d.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errHas)
			}
			if !strings.Contains(err.Error(), tc.errHas) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.errHas)
			}
		})
	}

	good := []WakeEntry{
		{Kind: WakeMention},
		{Kind: WakeTopic, Path: "a-b12"},
		{Kind: WakeSchedule, Name: "hourly", Pattern: "0 0 * * * *"},
		{Kind: WakeSchedule, Name: "once", Pattern: "@at 2027-01-01T00:00:00Z"},
		{Kind: WakeSubject, Subject: "acme.events.>"},
	}
	d := base
	d.Wake = good
	if err := d.Validate(); err != nil {
		t.Fatalf("valid wake set refused: %v", err)
	}
}

// FR-001: instructions, capabilities, and budget field invariants.
func TestInstructionsCapabilitiesBudgetValidation(t *testing.T) {
	base, _ := Parse([]byte(validJSON()))
	cases := []struct {
		name   string
		mutate func(*Declaration)
		errHas string
	}{
		{"instructions bad topic", func(d *Declaration) {
			d.Instructions = &Instructions{Topic: "Bad.Topic", Artefact: "x"}
		}, "invalid segment"},
		{"instructions empty artefact", func(d *Declaration) {
			d.Instructions = &Instructions{Topic: "a-b12", Artefact: "  "}
		}, "artefact reference is required"},
		{"capabilities bad role", func(d *Declaration) {
			d.Capabilities = &Capabilities{Role: "Not Valid"}
		}, "not a valid name"},
		{"capabilities bad tool", func(d *Declaration) {
			d.Capabilities = &Capabilities{Role: "ok", Tools: []string{"Bad Tool"}}
		}, "not a valid name"},
		{"capabilities duplicate tool", func(d *Declaration) {
			d.Capabilities = &Capabilities{Role: "ok", Tools: []string{"a", "a"}}
		}, "declared twice"},
		{"budget negative hops", func(d *Declaration) {
			d.Budget = &BudgetSpec{MaxHops: -1}
		}, "cannot be negative"},
		{"budget negative window", func(d *Declaration) {
			d.Budget = &BudgetSpec{Window: &WindowSpec{Max: -1}}
		}, "cannot be negative"},
		{"budget window without per", func(d *Declaration) {
			d.Budget = &BudgetSpec{Window: &WindowSpec{Max: 3}}
		}, "not a Go duration"},
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

// SC-005 lineage: strict decode reaches nested blocks — an unknown field
// inside a wake entry or instructions refuses the document.
func TestParseRejectsNestedUnknownFields(t *testing.T) {
	for name, doc := range map[string]string{
		"wake entry": `{"role":"agent","lifecycle":"service","persona":"p","topic":"t-ab12",
			"artifact":"file:///x","wake":[{"kind":"mention","cron":"@every 1s"}]}`,
		"instructions": `{"role":"agent","lifecycle":"service","persona":"p","topic":"t-ab12",
			"artifact":"file:///x","instructions":{"topic":"t-ab12","artefact":"x","pin":"abc"}}`,
		"budget": `{"role":"agent","lifecycle":"service","persona":"p","topic":"t-ab12",
			"artifact":"file:///x","budget":{"max_hops":4,"burst":2}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(doc)); err == nil {
				t.Fatal("expected the nested unknown field to be rejected")
			}
		})
	}
}

// FR-004: each kind carries its delivery class as a normative fact; topic
// types default to turn.post at read time (Validate never mutates).
func TestDeliveryClassAndEffectiveTypes(t *testing.T) {
	classes := map[WakeKind]string{
		WakeMention:  "replay-exact (notify stream, inbox-window bounded)",
		WakeTopic:    "replay-exact (ops stream)",
		WakeSchedule: "replay-exact (TTL-bounded backlog)",
		WakeSubject:  "at-most-once",
	}
	for kind, want := range classes {
		if got := (WakeEntry{Kind: kind}).DeliveryClass(); got != want {
			t.Errorf("DeliveryClass(%s) = %q, want %q", kind, got, want)
		}
	}
	e := WakeEntry{Kind: WakeTopic, Path: "a-b12"}
	if got := e.EffectiveTypes(); len(got) != 1 || got[0] != "turn.post" {
		t.Fatalf("EffectiveTypes default = %v, want [turn.post]", got)
	}
	e.Types = []string{"attachment.add"}
	if got := e.EffectiveTypes(); len(got) != 1 || got[0] != "attachment.add" {
		t.Fatalf("EffectiveTypes declared = %v", got)
	}
}

package wrap

import (
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulstream-core/realm"

	"github.com/impire-io/soulstream-workloads/declaration"
)

func agentDecl() declaration.Declaration {
	return declaration.Declaration{
		Role: declaration.RoleAgent, Lifecycle: declaration.LifecycleService,
		Persona: "clerk", Topic: "home-ab12", Artifact: "file:///opt/agents/clerk",
		Wake: []declaration.WakeEntry{
			{Kind: declaration.WakeMention},
			{Kind: declaration.WakeTopic, Path: "watched-ab12", Types: []string{"turn.post", "attachment.add"}},
			{Kind: declaration.WakeSchedule, Name: "daily", Pattern: "@every 24h", TTL: "1h"},
			{Kind: declaration.WakeSubject, Subject: "acme.events.>"},
		},
	}
}

// The full mapping: wake entries → the wake set, the declaration's topic →
// the home topic, the budget block → the 0006 knobs.
func TestDeclaredConfigMapsTheDeclaration(t *testing.T) {
	d := agentDecl()
	d.Budget = &declaration.BudgetSpec{MaxHops: 3, Window: &declaration.WindowSpec{Max: 5, Per: "2m"}}
	cfg, err := DeclaredConfig(Config{Persona: "clerk"}, d, nil)
	if err != nil {
		t.Fatalf("DeclaredConfig: %v", err)
	}
	if cfg.HomeTopic != "home-ab12" {
		t.Fatalf("home topic = %q", cfg.HomeTopic)
	}
	set := cfg.Wakes
	if set == nil || !set.Mention || len(set.Topics) != 1 || len(set.Schedules) != 1 || len(set.Subjects) != 1 {
		t.Fatalf("wake set = %+v", set)
	}
	if set.Topics[0].Path != "watched-ab12" || len(set.Topics[0].Types) != 2 {
		t.Fatalf("topic wake = %+v", set.Topics[0])
	}
	if s := set.Schedules[0]; s.Name != "daily" || s.Pattern != "@every 24h" || s.TTL != time.Hour {
		t.Fatalf("schedule wake = %+v", s)
	}
	if set.Subjects[0].Subject != "acme.events.>" {
		t.Fatalf("subject wake = %+v", set.Subjects[0])
	}
	want := Budget{MaxHops: 3, WindowMax: 5, WindowPer: 2 * time.Minute}
	if cfg.Budget != want {
		t.Fatalf("budget = %+v, want %+v", cfg.Budget, want)
	}
	if cfg.Unbudgeted {
		t.Fatal("a real budget must not read as unbudgeted")
	}
}

// Topic types default to turn.post through the mapping (EffectiveTypes).
func TestDeclaredConfigDefaultsTopicTypes(t *testing.T) {
	d := agentDecl()
	d.Wake = []declaration.WakeEntry{{Kind: declaration.WakeTopic, Path: "watched-ab12"}}
	cfg, err := DeclaredConfig(Config{Persona: "clerk"}, d, nil)
	if err != nil {
		t.Fatalf("DeclaredConfig: %v", err)
	}
	if got := cfg.Wakes.Topics[0].Types; len(got) != 1 || got[0] != "turn.post" {
		t.Fatalf("types = %v, want the turn.post default", got)
	}
}

// An explicit all-zero budget block is the operator's unbudgeted standing —
// never a gap for the defaults to refill.
func TestDeclaredConfigZeroBudgetIsUnbudgeted(t *testing.T) {
	d := agentDecl()
	d.Budget = &declaration.BudgetSpec{}
	cfg, err := DeclaredConfig(Config{Persona: "clerk"}, d, nil)
	if err != nil {
		t.Fatalf("DeclaredConfig: %v", err)
	}
	if !cfg.Unbudgeted {
		t.Fatal("an explicit zero budget must map to the unbudgeted standing")
	}
	cfg.ApplyDefaults()
	if cfg.Budget != (Budget{}) {
		t.Fatalf("defaults refilled an explicit zero budget: %+v", cfg.Budget)
	}
}

// An absent budget block leaves the engine defaults to apply.
func TestDeclaredConfigAbsentBudgetKeepsDefaults(t *testing.T) {
	cfg, err := DeclaredConfig(Config{Persona: "clerk"}, agentDecl(), nil)
	if err != nil {
		t.Fatalf("DeclaredConfig: %v", err)
	}
	cfg.ApplyDefaults()
	if cfg.Budget != (Budget{MaxHops: defaultMaxHops, WindowMax: defaultWindowMax, WindowPer: defaultWindowPer}) {
		t.Fatalf("budget = %+v, want the design defaults", cfg.Budget)
	}
}

// Refusals: persona mismatch, non-agent role, an empty wake set, and
// instructions without the engine's client.
func TestDeclaredConfigRefusals(t *testing.T) {
	cases := []struct {
		name   string
		base   Config
		mutate func(*declaration.Declaration)
		errHas string
	}{
		{"persona mismatch", Config{Persona: "other"}, nil, "does not match"},
		{"tool role", Config{Persona: "clerk"}, func(d *declaration.Declaration) {
			d.Role = declaration.RoleTool
			d.Wake = nil
		}, "only agent declarations"},
		{"empty wake set", Config{Persona: "clerk"}, func(d *declaration.Declaration) {
			d.Wake = nil
		}, "declares nothing"},
		{"instructions without client", Config{Persona: "clerk"}, func(d *declaration.Declaration) {
			d.Instructions = &declaration.Instructions{Topic: "souls-ab12", Artefact: "clerk.md"}
		}, "need the engine's realm client"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := agentDecl()
			if tc.mutate != nil {
				tc.mutate(&d)
			}
			_, err := DeclaredConfig(tc.base, d, nil)
			if err == nil || !strings.Contains(err.Error(), tc.errHas) {
				t.Fatalf("err = %v, want %q", err, tc.errHas)
			}
		})
	}
}

// Declared instructions wire the record source and grow the prompt with the
// {{INSTRUCTIONS}} placeholder when the template lacks it — and leave a
// template that already carries it untouched.
func TestDeclaredConfigInstructionsGrowThePrompt(t *testing.T) {
	d := agentDecl()
	d.Instructions = &declaration.Instructions{Topic: "souls-ab12", Artefact: "clerk.md"}
	client := &realm.Client{} // never dialed: the mapping only stores it

	base := Config{Persona: "clerk", Template: Template{Prompt: "answer as {{PERSONA}}"}}
	cfg, err := DeclaredConfig(base, d, client)
	if err != nil {
		t.Fatalf("DeclaredConfig: %v", err)
	}
	if cfg.Instructions == nil {
		t.Fatal("instructions source not wired")
	}
	if cfg.Template.Prompt != "answer as {{PERSONA}}\n\n{{INSTRUCTIONS}}" {
		t.Fatalf("prompt = %q, want the appended placeholder", cfg.Template.Prompt)
	}

	base.Template.Prompt = "{{INSTRUCTIONS}} then answer"
	cfg, err = DeclaredConfig(base, d, client)
	if err != nil {
		t.Fatalf("DeclaredConfig: %v", err)
	}
	if cfg.Template.Prompt != "{{INSTRUCTIONS}} then answer" {
		t.Fatalf("prompt = %q, want it untouched", cfg.Template.Prompt)
	}
}

// The wake-set validation the engine runs at startup: an empty set, a
// home-less non-record kind, and a non-replayable topic type all refuse.
func TestWakeSetValidate(t *testing.T) {
	var nilSet *WakeSet
	if err := nilSet.Validate(""); err != nil {
		t.Fatalf("nil set (legacy mention-only) must validate: %v", err)
	}
	if err := (&WakeSet{}).Validate("home"); err == nil {
		t.Fatal("an empty wake set must refuse")
	}
	if err := (&WakeSet{Schedules: []ScheduleWake{{Name: "d", Pattern: "@every 1s"}}}).Validate(""); err == nil {
		t.Fatal("a schedule wake without a home topic must refuse")
	}
	if err := (&WakeSet{Subjects: []SubjectWake{{Subject: "a.b"}}}).Validate(""); err == nil {
		t.Fatal("a subject wake without a home topic must refuse")
	}
	if err := (&WakeSet{Topics: []TopicWake{{Path: "p-ab12", Types: []string{"work.open"}}}}).Validate("home"); err == nil {
		t.Fatal("a non-replayable topic type must refuse")
	}
	ok := &WakeSet{Mention: true, Topics: []TopicWake{{Path: "p-ab12"}},
		Schedules: []ScheduleWake{{Name: "d", Pattern: "@every 1s"}}, Subjects: []SubjectWake{{Subject: "a.b"}}}
	if err := ok.Validate("home"); err != nil {
		t.Fatalf("a full valid set refused: %v", err)
	}
}

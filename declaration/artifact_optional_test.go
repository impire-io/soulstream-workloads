package declaration

import (
	"strings"
	"testing"
)

// The artifact becomes optional exactly where it was meaningless: an
// engine-served agent (role agent WITH a wake set) runs the node's harness
// template, never a declared executable — two independent consumers were
// writing file:///dev/null to satisfy the validator (design 0007 §9,
// closed). Everything else still declares what to run.
func TestArtifactOptionalForEngineServedAgents(t *testing.T) {
	base := Declaration{
		Role: RoleAgent, Lifecycle: LifecycleService,
		Persona: "clerk", Topic: "desk",
		Wake: []WakeEntry{{Kind: WakeMention}},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("engine-served agent without artifact refused: %v", err)
	}

	withArtifact := base
	withArtifact.Artifact = "file:///opt/agents/clerk"
	if err := withArtifact.Validate(); err != nil {
		t.Fatalf("engine-served agent WITH artifact refused: %v", err)
	}

	wakeless := base
	wakeless.Wake = nil
	if err := wakeless.Validate(); err == nil || !strings.Contains(err.Error(), "artifact") {
		t.Fatalf("a wake-less agent still needs its artifact: %v", err)
	}

	tool := Declaration{
		Role: RoleTool, Lifecycle: LifecycleService,
		Persona: "upper", Topic: "desk",
	}
	if err := tool.Validate(); err == nil || !strings.Contains(err.Error(), "artifact") {
		t.Fatalf("a tool still needs its artifact: %v", err)
	}
}

// The engine's defaults are readable where every consumer already looks —
// a surface showing the bounds a person runs under and the engine
// enforcing them cannot drift, because there is exactly one value.
func TestDefaultBudgetIsTheSource(t *testing.T) {
	if DefaultBudget.MaxHops != 4 || DefaultBudget.Window == nil ||
		DefaultBudget.Window.Max != 8 || DefaultBudget.Window.Per != "10m" {
		t.Fatalf("the shipped defaults moved without their episode: %+v", DefaultBudget)
	}
}

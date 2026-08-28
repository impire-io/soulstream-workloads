package fleet

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/declaration"
)

// TestPlacementRoundTripsCapabilities (spec 010 SC-004): a capability-bearing
// declaration survives the placement encoding — the exact body Submit writes
// — and comes back intact through DeclarationOf, so the winning node launches
// with the same selectors the submitter declared.
func TestPlacementRoundTripsCapabilities(t *testing.T) {
	d := declaration.Declaration{
		Role:      declaration.RoleAgent,
		Lifecycle: declaration.LifecycleService,
		Persona:   "sprite",
		Topic:     "t-ab12",
		Artifact:  "file:///bin/true",
		Capabilities: &declaration.Capabilities{
			Role:  "agent",
			Tools: []string{"toola", "toolb"},
		},
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("declaration: %v", err)
	}
	body, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	got, ok := DeclarationOf(topic.WorkItem{Body: submissionMarker + string(body)})
	if !ok {
		t.Fatal("DeclarationOf did not recognize the submission")
	}
	if !reflect.DeepEqual(got, d) {
		t.Fatalf("round trip changed the declaration:\n got %+v\nwant %+v", got, d)
	}
}

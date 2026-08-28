package declaration

import (
	"strings"
	"testing"
)

// The inference block (hq design 0007 §3 closing against the plane's
// catalogue): a virtual NAME, agent-only, never a credential — and the
// strict parse keeps every other field out of the block by construction.
func TestInferenceBlock(t *testing.T) {
	base := `{"role":"agent","lifecycle":"service","persona":"clerk",
		"topic":"desk","artifact":"file:///bin/true",
		"wake":[{"kind":"mention"}],`

	d, err := Parse([]byte(base + `"inference":{"model":"realm-default"}}`))
	if err != nil {
		t.Fatalf("named inference refused: %v", err)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if d.Inference.Model != "realm-default" {
		t.Fatalf("model = %q", d.Inference.Model)
	}

	if _, err := Parse([]byte(base + `"inference":{"model":"m","api_key":"sk-x"}}`)); err == nil {
		t.Fatal("a credential field rode the inference block")
	}

	d, err = Parse([]byte(base + `"inference":{"model":""}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("empty model: %v", err)
	}

	d, err = Parse([]byte(base + `"inference":{"model":"sk-oops-a-key"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(); err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("credential-shaped model: %v", err)
	}

	tool := `{"role":"tool","lifecycle":"service","persona":"upper",
		"topic":"desk","artifact":"file:///bin/true",
		"inference":{"model":"m"}}`
	d, err = Parse([]byte(tool))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(); err == nil || !strings.Contains(err.Error(), "agent-only") {
		t.Fatalf("tool with inference: %v", err)
	}
}

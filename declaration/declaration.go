// Package declaration is the operator-facing workload contract: parsing and
// validation of a workload declaration. It is pure — it imports no NATS — so it
// unit-tests with no server.
//
// A declaration says WHAT to run and AS WHOM, never HOW it is isolated: there is
// deliberately no backend field, and one is rejected if present (constitution
// III — contracts orthogonal to backends).
package declaration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/impire-io/soulstream-core/identity"
)

// Role is what a workload is to the realm.
type Role string

// Lifecycle is how the runtime schedules a workload.
type Lifecycle string

const (
	// RoleAgent is a long-lived persona that participates in topics.
	RoleAgent Role = "agent"
	// RoleTool is a capability other workloads call. Not accepted in M1.1.
	RoleTool Role = "tool"

	// LifecycleService is long-lived; runs until stopped.
	LifecycleService Lifecycle = "service"
	// LifecycleFunction is short-lived; triggered on demand. Not accepted in M1.1.
	LifecycleFunction Lifecycle = "function"
	// LifecycleJob runs to completion once. Not accepted in M1.1.
	LifecycleJob Lifecycle = "job"
)

// Declaration is a single workload's contract.
type Declaration struct {
	Role      Role      `json:"role"`
	Lifecycle Lifecycle `json:"lifecycle"`
	Persona   string    `json:"persona"`
	Topic     string    `json:"topic"`
	Artifact  string    `json:"artifact"`
	Args      []string  `json:"args,omitempty"`
}

// Parse decodes a declaration from JSON. Decoding is strict: an unknown field
// (including any backend-specific key) fails loud, so a declaration can never
// smuggle a backend hint past validation (SC-005).
func Parse(data []byte) (Declaration, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var d Declaration
	if err := dec.Decode(&d); err != nil {
		return Declaration{}, fmt.Errorf("parse declaration: %w", err)
	}
	if dec.More() {
		return Declaration{}, fmt.Errorf("parse declaration: unexpected trailing content")
	}
	return d, nil
}

// Validate enforces the M1.1-accepted subset and the field invariants. It never
// mutates the declaration.
func (d Declaration) Validate() error {
	switch d.Role {
	case RoleAgent, RoleTool:
	default:
		return fmt.Errorf("role %q is not a known role", d.Role)
	}

	switch d.Lifecycle {
	case LifecycleService:
	case LifecycleFunction, LifecycleJob:
		return fmt.Errorf("lifecycle %q not supported yet (M1.1 accepts %q)", d.Lifecycle, LifecycleService)
	default:
		return fmt.Errorf("lifecycle %q is not a known lifecycle", d.Lifecycle)
	}

	if !identity.ValidName(d.Persona) {
		return fmt.Errorf("persona %q is not a valid persona name", d.Persona)
	}

	if err := validateTopicPath(d.Topic); err != nil {
		return err
	}

	if err := validateArtifact(d.Artifact); err != nil {
		return err
	}

	return nil
}

// validateTopicPath accepts a soulstream topic path: dot-separated segments,
// each a valid soulstream name (e.g. "q2-planning-ab12" or the nested
// "acme-team.q2-planning-ab12"). The dot is soulstream's path separator
// (topic.ChildPath), which also makes each segment a subject token.
func validateTopicPath(path string) error {
	if path == "" {
		return fmt.Errorf("topic is required")
	}
	segs := strings.Split(path, ".")
	for _, s := range segs {
		if !identity.ValidName(s) {
			return fmt.Errorf("topic %q has an invalid segment %q", path, s)
		}
	}
	return nil
}

// validateArtifact requires a file:// URI for M1.1; nats:// object-store
// artifacts arrive with a later backend.
func validateArtifact(artifact string) error {
	if artifact == "" {
		return fmt.Errorf("artifact is required")
	}
	u, err := url.Parse(artifact)
	if err != nil {
		return fmt.Errorf("artifact %q is not a valid URI: %w", artifact, err)
	}
	if u.Scheme != "file" {
		return fmt.Errorf("artifact scheme %q not supported yet (M1.1 accepts file://)", u.Scheme)
	}
	if u.Path == "" {
		return fmt.Errorf("artifact %q has no path", artifact)
	}
	return nil
}

// ArtifactPath returns the local filesystem path of a validated file:// artifact.
func (d Declaration) ArtifactPath() (string, error) {
	u, err := url.Parse(d.Artifact)
	if err != nil {
		return "", fmt.Errorf("artifact %q is not a valid URI: %w", d.Artifact, err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("artifact scheme %q is not file://", u.Scheme)
	}
	return u.Path, nil
}

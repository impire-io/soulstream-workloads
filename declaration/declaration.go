// Package declaration is the operator-facing workload contract: parsing and
// validation of a workload declaration. It is pure — it imports no NATS — so it
// unit-tests with no server.
//
// A declaration says WHAT to run and AS WHOM, never HOW it is isolated: there is
// deliberately no backend field, and one is rejected if present (constitution
// III — contracts orthogonal to backends).
//
// An agent declaration (hq design 0005) additionally says what INSTRUCTS the
// agent, what it MAY REACH, and what WAKES it. Instructions and the record-form
// artifact are references into the record (a stage-1 artefact lineage — the
// runtime materialises the tip, never a durable copy). Capabilities are names,
// not grants: the role names a vault entry, the tools ride as mint tags for the
// account's scoped template to resolve; nothing here widens anything (the
// identity-backed resolution is the named follow-on feature capability-minting).
// Wake entries each carry a delivery class as a normative fact readers and
// shells surface — declaring the at-most-once subject kind is declaring its
// loss honestly.
package declaration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

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
	Artifact  string    `json:"artifact,omitempty"`
	Args      []string  `json:"args,omitempty"`

	// Instructions references a stage-1 artefact lineage whose tip is the
	// agent's current instructions — materialised at every wake, digest-checked,
	// never held durably. Agent-only.
	Instructions *Instructions `json:"instructions,omitempty"`
	// Capabilities are names, not grants (hq design 0005 §5): a vault-entry
	// role name plus tool names that ride as mint tags. Agent-only; resolution
	// is the capability-minting follow-on.
	Capabilities *Capabilities `json:"capabilities,omitempty"`
	// Wake declares what wakes the agent — one entry per source, each carrying
	// its delivery class as a normative fact. Agent-only.
	Wake []WakeEntry `json:"wake,omitempty"`
	// Budget is the 0006 wake budget the engine admits wakes under. Agent-only;
	// absent means the engine's defaults.
	Budget *BudgetSpec `json:"budget,omitempty"`
	// Inference names what serves the agent's thinking (hq design 0007 §3
	// closing against soulstream-inference design 0001 §5): a VIRTUAL model
	// name the deployment's catalogue resolves — never a concrete model, never
	// a provider, never a credential. Absent means the ambient lane (the
	// harness's own authentication, wrap's founding shape). Agent-only;
	// resolution is the dispatcher's product wiring.
	Inference *InferenceSpec `json:"inference,omitempty"`
}

// DefaultBudget is the wake budget the engine applies when a declaration
// carries none (design 0006's shipped defaults: depth 4, window 8 per 10
// minutes). Exported here — beside the schema, in the package every
// consumer already reads — because a surface that must SHOW the bounds a
// person runs under needs the numbers from the source, not a restatement
// that drifts silently. wrap consumes this value, so there is exactly one.
var DefaultBudget = BudgetSpec{
	MaxHops: 4,
	Window:  &WindowSpec{Max: 8, Per: "10m"},
}

// InferenceSpec is the declaration's inference requirement: a name, not a
// route and not a grant. The strict parse refuses any other field here, so
// a credential cannot ride the block by construction.
type InferenceSpec struct {
	Model string `json:"model"`
}

// Instructions is a reference to an artefact lineage on a topic.
type Instructions struct {
	Topic    string `json:"topic"`
	Artefact string `json:"artefact"`
}

// Capabilities names what the agent may reach: a scoped signing key by vault
// entry name, and tool names resolved by the account's scoped template. The
// declaration cannot widen anything — these are selectors, not grants.
type Capabilities struct {
	Role  string   `json:"role"`
	Tools []string `json:"tools,omitempty"`
}

// WakeKind is one of the four declared wake sources.
type WakeKind string

// The wake kinds (hq design 0005 §2-3).
const (
	WakeMention  WakeKind = "mention"
	WakeTopic    WakeKind = "topic"
	WakeSchedule WakeKind = "schedule"
	WakeSubject  WakeKind = "subject"
)

// WakeEntry is one declared wake source. Exactly the fields of its kind may be
// set; anything foreign to the kind refuses the declaration.
type WakeEntry struct {
	Kind WakeKind `json:"kind"`

	// topic kind
	Path  string   `json:"path,omitempty"`
	Types []string `json:"types,omitempty"` // default [turn.post] — see EffectiveTypes

	// schedule kind
	Name    string `json:"name,omitempty"`
	Pattern string `json:"pattern,omitempty"` // @every <dur> | @at <RFC3339 UTC> | 6-field cron
	TTL     string `json:"ttl,omitempty"`     // optional positive Go duration — the tick backlog bound

	// subject kind
	Subject string `json:"subject,omitempty"`
}

// EffectiveTypes returns the op types a topic wake fires on: the declared
// types, or the default [turn.post]. Validate never mutates, so the default is
// applied here at read time.
func (e WakeEntry) EffectiveTypes() []string {
	if len(e.Types) > 0 {
		return e.Types
	}
	return []string{"turn.post"}
}

// DeliveryClass is the kind's delivery guarantee — a normative fact readers
// and shells MUST surface (hq design 0005 §2). Declaring the subject kind is
// declaring its loss honestly: a wake arriving while the agent is down is lost.
func (e WakeEntry) DeliveryClass() string {
	switch e.Kind {
	case WakeMention:
		return "replay-exact (notify stream, inbox-window bounded)"
	case WakeTopic:
		return "replay-exact (ops stream)"
	case WakeSchedule:
		return "replay-exact (TTL-bounded backlog)"
	case WakeSubject:
		return "at-most-once"
	default:
		return ""
	}
}

// BudgetSpec is the declaration's wake-budget block (hq design 0006 §3): the
// provable-chain depth bound and the authorship-window floor. A zero knob
// disables that part; an absent block means the engine's defaults.
type BudgetSpec struct {
	MaxHops int         `json:"max_hops"`
	Window  *WindowSpec `json:"window,omitempty"`
}

// WindowSpec is the window floor: at most Max own turn.posts per topic per Per.
type WindowSpec struct {
	Max int    `json:"max"`
	Per string `json:"per"` // Go duration
}

// Parse decodes a declaration from JSON. Decoding is strict: an unknown field
// (including any backend-specific key, nested blocks included) fails loud, so a
// declaration can never smuggle a backend hint past validation (SC-005).
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

// Validate enforces the accepted subset and the field invariants. It never
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

	if err := ValidateTopicPath(d.Topic); err != nil {
		return err
	}

	// An engine-served agent (role agent WITH a wake set) runs the node's
	// harness template, never a declared executable — requiring an artifact
	// there made every screen and CLI write a dummy value to satisfy a
	// validator (design 0007 §9's open, closed 2026-08-28: two independent
	// consumers were writing file:///dev/null). Everything else still
	// declares what to run.
	engineServed := d.Role == RoleAgent && len(d.Wake) > 0
	if d.Artifact != "" || !engineServed {
		if err := validateArtifact(d.Artifact); err != nil {
			return err
		}
	}

	if d.Role != RoleAgent {
		if d.Instructions != nil || d.Capabilities != nil || len(d.Wake) > 0 || d.Budget != nil || d.Inference != nil {
			return fmt.Errorf("instructions, capabilities, wake, budget and inference are agent-only (role is %q)", d.Role)
		}
		return nil
	}

	if d.Instructions != nil {
		if err := d.Instructions.validate(); err != nil {
			return err
		}
	}
	if d.Capabilities != nil {
		if err := d.Capabilities.Validate(); err != nil {
			return err
		}
	}
	if err := validateWake(d.Wake); err != nil {
		return err
	}
	if d.Budget != nil {
		if err := d.Budget.validate(); err != nil {
			return err
		}
	}
	if d.Inference != nil {
		if err := d.Inference.validate(); err != nil {
			return err
		}
	}

	return nil
}

func (i InferenceSpec) validate() error {
	if i.Model == "" {
		return fmt.Errorf("inference.model is required when the block is present — a name the catalogue resolves")
	}
	// The design's refusal by name (hq 0007 §3): a credential is not a
	// model name, and the classic paste is caught where it happens.
	if strings.HasPrefix(i.Model, "sk-") {
		return fmt.Errorf("inference.model %q looks like a credential — the declaration names, custody grants", i.Model)
	}
	return nil
}

func (i Instructions) validate() error {
	if err := ValidateTopicPath(i.Topic); err != nil {
		return fmt.Errorf("instructions: %w", err)
	}
	if strings.TrimSpace(i.Artefact) == "" {
		return fmt.Errorf("instructions: an artefact reference is required")
	}
	return nil
}

// Validate enforces the capability grammar: the role a valid vault-entry
// name, every tool a valid name, no duplicates. Exported as the single
// source of that grammar — the minter re-validates at its own boundary
// (a public surface must not rely on its callers).
func (c Capabilities) Validate() error {
	if !identity.ValidName(c.Role) {
		return fmt.Errorf("capabilities: role %q is not a valid name (a vault entry name)", c.Role)
	}
	seen := map[string]bool{}
	for _, t := range c.Tools {
		if !identity.ValidName(t) {
			return fmt.Errorf("capabilities: tool %q is not a valid name", t)
		}
		if seen[t] {
			return fmt.Errorf("capabilities: tool %q is declared twice", t)
		}
		seen[t] = true
	}
	return nil
}

// validateWake checks each entry per kind and refuses duplicates that would
// silently collapse (one mention, unique topic paths, unique schedule names —
// two registrations on one name would replace each other — unique subjects).
func validateWake(entries []WakeEntry) error {
	mentions := 0
	paths := map[string]bool{}
	names := map[string]bool{}
	subjects := map[string]bool{}
	for i, e := range entries {
		if err := e.validate(); err != nil {
			return fmt.Errorf("wake[%d]: %w", i, err)
		}
		switch e.Kind {
		case WakeMention:
			mentions++
			if mentions > 1 {
				return fmt.Errorf("wake[%d]: a second mention entry declares nothing new", i)
			}
		case WakeTopic:
			if paths[e.Path] {
				return fmt.Errorf("wake[%d]: topic path %q is declared twice", i, e.Path)
			}
			paths[e.Path] = true
		case WakeSchedule:
			if names[e.Name] {
				return fmt.Errorf("wake[%d]: schedule name %q is declared twice (a re-published registration replaces)", i, e.Name)
			}
			names[e.Name] = true
		case WakeSubject:
			if subjects[e.Subject] {
				return fmt.Errorf("wake[%d]: subject %q is declared twice", i, e.Subject)
			}
			subjects[e.Subject] = true
		}
	}
	return nil
}

func (e WakeEntry) validate() error {
	foreign := func(kind WakeKind, set map[string]string) error {
		for field, val := range set {
			if val != "" {
				return fmt.Errorf("%s wake does not take %s", kind, field)
			}
		}
		return nil
	}
	switch e.Kind {
	case WakeMention:
		if len(e.Types) > 0 {
			return fmt.Errorf("mention wake does not take types")
		}
		return foreign(e.Kind, map[string]string{
			"path": e.Path, "name": e.Name, "pattern": e.Pattern, "ttl": e.TTL, "subject": e.Subject,
		})
	case WakeTopic:
		if err := foreign(e.Kind, map[string]string{
			"name": e.Name, "pattern": e.Pattern, "ttl": e.TTL, "subject": e.Subject,
		}); err != nil {
			return err
		}
		if err := ValidateTopicPath(e.Path); err != nil {
			return fmt.Errorf("topic wake: %w", err)
		}
		for _, t := range e.Types {
			if err := validateOpType(t); err != nil {
				return fmt.Errorf("topic wake: %w", err)
			}
		}
		return nil
	case WakeSchedule:
		if len(e.Types) > 0 {
			return fmt.Errorf("schedule wake does not take types")
		}
		if err := foreign(e.Kind, map[string]string{
			"path": e.Path, "subject": e.Subject,
		}); err != nil {
			return err
		}
		if !identity.ValidName(e.Name) {
			return fmt.Errorf("schedule wake: name %q is not a valid name (it becomes a subject token)", e.Name)
		}
		if err := validatePattern(e.Pattern); err != nil {
			return fmt.Errorf("schedule wake %q: %w", e.Name, err)
		}
		if e.TTL != "" {
			ttl, err := time.ParseDuration(e.TTL)
			if err != nil {
				return fmt.Errorf("schedule wake %q: ttl %q is not a Go duration: %w", e.Name, e.TTL, err)
			}
			if ttl <= 0 {
				return fmt.Errorf("schedule wake %q: ttl must be positive, got %s", e.Name, ttl)
			}
		}
		return nil
	case WakeSubject:
		if len(e.Types) > 0 {
			return fmt.Errorf("subject wake does not take types")
		}
		if err := foreign(e.Kind, map[string]string{
			"path": e.Path, "name": e.Name, "pattern": e.Pattern, "ttl": e.TTL,
		}); err != nil {
			return err
		}
		return validateSubject(e.Subject)
	default:
		return fmt.Errorf("kind %q is not a known wake kind (mention, topic, schedule, subject)", e.Kind)
	}
}

// validatePattern accepts the substrate's schedule grammar: "@every <Go
// duration>", "@at <RFC3339 UTC>", or a 6-field cron expression.
func validatePattern(p string) error {
	if p == "" {
		return fmt.Errorf("a pattern is required (@every <duration>, @at <RFC3339>, or 6-field cron)")
	}
	if rest, ok := strings.CutPrefix(p, "@every "); ok {
		d, err := time.ParseDuration(rest)
		if err != nil {
			return fmt.Errorf("pattern %q: %q is not a Go duration: %w", p, rest, err)
		}
		if d <= 0 {
			return fmt.Errorf("pattern %q: the interval must be positive", p)
		}
		return nil
	}
	if rest, ok := strings.CutPrefix(p, "@at "); ok {
		if _, err := time.Parse(time.RFC3339, rest); err != nil {
			return fmt.Errorf("pattern %q: %q is not an RFC3339 time: %w", p, rest, err)
		}
		return nil
	}
	if strings.HasPrefix(p, "@") {
		return fmt.Errorf("pattern %q: unknown @-form (accepted: @every, @at)", p)
	}
	fields := strings.Fields(p)
	if len(fields) != 6 {
		return fmt.Errorf("pattern %q: a cron expression needs 6 fields, got %d", p, len(fields))
	}
	for _, f := range fields {
		for _, r := range f {
			switch {
			case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			case r == '*' || r == ',' || r == '/' || r == '-':
			default:
				return fmt.Errorf("pattern %q: cron field %q holds %q", p, f, string(r))
			}
		}
	}
	return nil
}

// validateSubject accepts a plain NATS subscription subject: dot-separated
// non-empty tokens, wildcards allowed ('*' as a whole token, '>' only last).
func validateSubject(s string) error {
	if s == "" {
		return fmt.Errorf("subject wake: a subject is required")
	}
	toks := strings.Split(s, ".")
	for i, tok := range toks {
		if tok == "" {
			return fmt.Errorf("subject wake: subject %q has an empty token", s)
		}
		if strings.ContainsAny(tok, " \t") {
			return fmt.Errorf("subject wake: subject %q holds whitespace", s)
		}
		if tok == ">" && i != len(toks)-1 {
			return fmt.Errorf("subject wake: subject %q uses '>' before the last token", s)
		}
	}
	return nil
}

// validateOpType accepts an op-type name: dot-separated lowercase segments
// (e.g. "turn.post", "attachment.add").
func validateOpType(t string) error {
	if t == "" {
		return fmt.Errorf("an op type must not be empty")
	}
	for _, seg := range strings.Split(t, ".") {
		if seg == "" {
			return fmt.Errorf("op type %q has an empty segment", t)
		}
		for _, r := range seg {
			switch {
			case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-' || r == '_':
			default:
				return fmt.Errorf("op type %q holds %q", t, string(r))
			}
		}
	}
	return nil
}

func (b BudgetSpec) validate() error {
	if b.MaxHops < 0 {
		return fmt.Errorf("budget: max_hops cannot be negative, got %d", b.MaxHops)
	}
	if b.Window != nil {
		if b.Window.Max < 0 {
			return fmt.Errorf("budget: window.max cannot be negative, got %d", b.Window.Max)
		}
		if b.Window.Max > 0 {
			per, err := time.ParseDuration(b.Window.Per)
			if err != nil {
				return fmt.Errorf("budget: window.per %q is not a Go duration: %w", b.Window.Per, err)
			}
			if per <= 0 {
				return fmt.Errorf("budget: window.per must be positive, got %s", per)
			}
		}
	}
	return nil
}

// ValidateTopicPath accepts a soulstream topic path: dot-separated segments,
// each a valid soulstream name (e.g. "q2-planning-ab12" or the nested
// "acme-team.q2-planning-ab12"). The dot is soulstream's path separator
// (topic.ChildPath), which also makes each segment a subject token.
// Exported as the single source of the path grammar (the minter's tag
// rendering checks values against it).
func ValidateTopicPath(path string) error {
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

// The artifact schemes: file:// is a host path (native workloads); the record
// form soulstream://<topic-path>/<artefact-name> names an artefact lineage in
// the record — the runtime materialises the tip per run, digest-checked, never
// a durable copy. Declared agents boot from the record form; nats:// object
// store stays out.
const (
	SchemeFile       = "file"
	SchemeSoulstream = "soulstream"
)

const soulstreamPrefix = SchemeSoulstream + "://"

// validateArtifact requires a file:// URI or the record form
// soulstream://<topic-path>/<artefact-name>.
func validateArtifact(artifact string) error {
	if artifact == "" {
		return fmt.Errorf("artifact is required")
	}
	if strings.HasPrefix(artifact, soulstreamPrefix) {
		_, err := parseRecordArtifact(artifact)
		return err
	}
	u, err := url.Parse(artifact)
	if err != nil {
		return fmt.Errorf("artifact %q is not a valid URI: %w", artifact, err)
	}
	if u.Scheme != SchemeFile {
		return fmt.Errorf("artifact scheme %q not supported (file://, soulstream://)", u.Scheme)
	}
	if u.Path == "" {
		return fmt.Errorf("artifact %q has no path", artifact)
	}
	return nil
}

// ArtifactRef is a parsed, validated artifact reference.
type ArtifactRef struct {
	Scheme string // SchemeFile | SchemeSoulstream
	Path   string // file: the local filesystem path
	Topic  string // soulstream: the topic path holding the lineage
	Name   string // soulstream: the artefact name (lineage root or tip name)
}

// ArtifactRef parses the declaration's artifact into its scheme's parts.
func (d Declaration) ArtifactRef() (ArtifactRef, error) {
	if strings.HasPrefix(d.Artifact, soulstreamPrefix) {
		return parseRecordArtifact(d.Artifact)
	}
	p, err := d.ArtifactPath()
	if err != nil {
		return ArtifactRef{}, err
	}
	return ArtifactRef{Scheme: SchemeFile, Path: p}, nil
}

// parseRecordArtifact splits soulstream://<topic-path>/<artefact-name>. The
// topic path validates by the path grammar; the name must be non-empty and
// slash-free (names are labels — an ambiguous label refuses at resolution,
// where the lineage roots can be named).
func parseRecordArtifact(artifact string) (ArtifactRef, error) {
	rest := strings.TrimPrefix(artifact, soulstreamPrefix)
	topicPath, name, ok := strings.Cut(rest, "/")
	if !ok || name == "" {
		return ArtifactRef{}, fmt.Errorf("artifact %q wants the form %s<topic-path>/<artefact-name>", artifact, soulstreamPrefix)
	}
	if strings.Contains(name, "/") {
		return ArtifactRef{}, fmt.Errorf("artifact %q: the artefact name %q must not hold '/'", artifact, name)
	}
	if err := ValidateTopicPath(topicPath); err != nil {
		return ArtifactRef{}, fmt.Errorf("artifact %q: %w", artifact, err)
	}
	return ArtifactRef{Scheme: SchemeSoulstream, Topic: topicPath, Name: name}, nil
}

// ArtifactPath returns the local filesystem path of a validated file:// artifact.
func (d Declaration) ArtifactPath() (string, error) {
	u, err := url.Parse(d.Artifact)
	if err != nil {
		return "", fmt.Errorf("artifact %q is not a valid URI: %w", d.Artifact, err)
	}
	if u.Scheme != SchemeFile {
		return "", fmt.Errorf("artifact scheme %q is not file://", u.Scheme)
	}
	return u.Path, nil
}

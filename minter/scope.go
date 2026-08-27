// Package minter issues per-workload NATS credentials scoped to a persona's
// realm subjects. The scope construction is pure (no NATS, no signing) and
// unit-tests without a server; only Mint signs, using the realm-account key.
//
// The design borrows NEX's CredVendor shape (a fresh user per workload,
// delivered as JWT + seed) but replaces NEX's operational scope with a
// realm-semantic one: a persona may act only on its topic's subjects, and a
// tool only on its service subject.
package minter

import (
	"fmt"

	"github.com/impire-io/soulstream-core/identity"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/declaration"
)

// Realm-subject building blocks. Topic/notify subjects mirror soulstream's
// taxonomy (they are stored ops on the SOULSTREAM.> stream). Tool request-reply
// is TRANSIENT — not a stored op — so it lives on soulstream-workloads's own SOULSTREAM.SVC.*
// namespace, which the soulstream stream deliberately does not capture (else a
// JetStream ack would race the tool's reply).
const (
	notifyPrefix   = "SOULSTREAM.PERSONA.NOTIFY."
	notifyWildcard = notifyPrefix + "*"
	svcPrefix      = "SOULSTREAM.SVC."
	svcWildcard    = svcPrefix + ">"
	inboxWildcard  = "_INBOX.>"
	// jsAPIInfo is the one JetStream API subject an agent needs:
	// realm.NewClient's availability probe (js.AccountInfo). Found by the
	// first enforcing consumer (soulnode M1.3) — the open-server suites
	// never noticed its absence; without it the reference agent's own
	// realm client refuses to construct under operator mode.
	jsAPIInfo = "$JS.API.INFO"
)

// Scope is the pure description of what a persona may touch, by role.
type Scope struct {
	Role    declaration.Role // agent (default) or tool
	Persona string
	Topic   string // the topic the workload's lifecycle lives on / an agent participates in

	// Capabilities are the declaration's capability selectors (names, not
	// grants — hq design 0005 §5). nil reproduces today's derivation
	// byte-for-byte; set, the tool namespace narrows to exactly the
	// declared tools and MintTags renders the external minter's tag list.
	// Agent-only.
	Capabilities *declaration.Capabilities
}

// Tag keys — the identity plane's mint-tag vocabulary (hq designs 0005 §5
// and 0003-fleet §5): values an account's scoped templates resolve
// ({{tag(tool)}} and siblings). The identity repo cannot import this one
// (the cycle guard — consumers wire the seam), so the same key names are
// declared on its side; the product's consumer-position e2e is the drift
// court.
const (
	TagKeyPersona = "persona"
	TagKeyTopic   = "topic"
	TagKeyTool    = "tool"
)

// PermissionSet is the pub/sub allow-lists derived from a Scope.
type PermissionSet struct {
	Pub []string
	Sub []string
}

// PermissionSet derives the realm-semantic allow-lists for the scope. Pure:
// same input, same output, no I/O.
//
// An agent participating in topic T may publish its turns/work ops on T, mention
// others, CALL tools (SOULSTREAM.SVC.>), and reply on its inbox; and follow T,
// receive on its own notify inbox and reply inbox.
//
// A tool may only SERVE its capability: subscribe its own service subject
// (SOULSTREAM.SVC.<persona>) and reply on inboxes — nothing else.
//
// Everything not listed is denied by omission; the operator-mode enforcement
// test asserts an out-of-scope publish is refused by the server.
func (s Scope) PermissionSet() PermissionSet {
	if s.Role == declaration.RoleTool {
		return PermissionSet{
			Pub: []string{inboxWildcard},
			Sub: []string{svcPrefix + s.Persona, inboxWildcard},
		}
	}
	ops := topic.OpsSubject(s.Topic)
	pub := []string{ops, notifyWildcard}
	if s.Capabilities == nil {
		// No capabilities declared: today's grant, byte-identical — any
		// tool by name.
		pub = append(pub, svcWildcard)
	} else {
		// Capabilities narrow the tool namespace to exactly the declared
		// tools (design 0005 §5); an empty list grants none at all.
		for _, t := range s.Capabilities.Tools {
			pub = append(pub, svcPrefix+t)
		}
	}
	pub = append(pub, inboxWildcard, jsAPIInfo)
	return PermissionSet{
		Pub: pub,
		Sub: []string{ops, topic.InfoSubjectWildcard, notifyPrefix + s.Persona, inboxWildcard},
	}
}

// MintTags renders the scope as identity-plane mint tags — persona, topic,
// then one tool tag per declared tool, in declaration order. A scope
// without capabilities renders no tags (nil, nil): tags exist for the
// external minter's tag-template lane only. Every value is checked against
// the shared name/path grammar, so a corrupted scope can never smuggle
// subject grammar into a template expansion.
func (s Scope) MintTags() ([]string, error) {
	if s.Capabilities == nil {
		return nil, nil
	}
	if err := s.validateCapabilities(); err != nil {
		return nil, err
	}
	if !identity.ValidName(s.Persona) {
		return nil, fmt.Errorf("minter: persona %q is not a valid name", s.Persona)
	}
	if err := declaration.ValidateTopicPath(s.Topic); err != nil {
		return nil, fmt.Errorf("minter: %w", err)
	}
	tags := []string{
		TagKeyPersona + ":" + s.Persona,
		TagKeyTopic + ":" + s.Topic,
	}
	for _, t := range s.Capabilities.Tools {
		tags = append(tags, TagKeyTool+":"+t)
	}
	return tags, nil
}

// validateCapabilities re-refuses invalid capability selectors at the mint
// boundary. Declarations already validate; the minter is a public surface
// and must not rely on its callers (defense in depth).
func (s Scope) validateCapabilities() error {
	if s.Capabilities == nil {
		return nil
	}
	if s.Role == declaration.RoleTool {
		return fmt.Errorf("minter: capabilities are agent-only (role is %q)", s.Role)
	}
	if err := s.Capabilities.Validate(); err != nil {
		return fmt.Errorf("minter: %w", err)
	}
	return nil
}

// ServiceSubject is the request-reply subject a tool serves on, derived from its
// name — this is how a caller "discovers" a tool by name.
func ServiceSubject(toolPersona string) string { return svcPrefix + toolPersona }

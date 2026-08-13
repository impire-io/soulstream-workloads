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
}

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
	return PermissionSet{
		Pub: []string{ops, notifyWildcard, svcWildcard, inboxWildcard, jsAPIInfo},
		Sub: []string{ops, topic.InfoSubjectWildcard, notifyPrefix + s.Persona, inboxWildcard},
	}
}

// ServiceSubject is the request-reply subject a tool serves on, derived from its
// name — this is how a caller "discovers" a tool by name.
func ServiceSubject(toolPersona string) string { return svcPrefix + toolPersona }

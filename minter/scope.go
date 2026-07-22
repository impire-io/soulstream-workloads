// Package minter issues per-workload NATS credentials scoped to a persona's
// realm subjects. The scope construction is pure (no NATS, no signing) and
// unit-tests without a server; only Mint signs, using the realm-account key.
//
// The design borrows NEX's CredVendor shape (a fresh user per workload,
// delivered as JWT + seed) but replaces NEX's operational scope with a
// realm-semantic one: the persona may act only on its topic's subjects.
package minter

import "github.com/impire-io/soulstream/topic"

// Realm-subject building blocks (mirroring soulstream's taxonomy).
const (
	notifyPrefix   = "SOULSTREAM.PERSONA.NOTIFY."
	notifyWildcard = notifyPrefix + "*"
	inboxWildcard  = "_INBOX.>"
)

// Scope is the pure description of what a persona may touch on one topic.
type Scope struct {
	Persona string // the persona the workload runs as
	Topic   string // the topic path the persona participates in
}

// PermissionSet is the pub/sub allow-lists derived from a Scope.
type PermissionSet struct {
	Pub []string
	Sub []string
}

// PermissionSet derives the realm-semantic allow-lists for the scope. Pure:
// same input, same output, no I/O.
//
// An agent P participating in topic T may:
//   - publish its turns/work ops on the topic's OPS subject, mention others on
//     the notify subjects, and reply on its inbox;
//   - follow the topic (OPS + INFO), receive on its own notify inbox, and its
//     reply inbox.
//
// Everything else is denied by omission; SC-003 asserts a publish outside this
// set is refused by the server.
func (s Scope) PermissionSet() PermissionSet {
	ops := topic.OpsSubject(s.Topic)
	return PermissionSet{
		Pub: []string{
			ops,
			notifyWildcard,
			inboxWildcard,
		},
		Sub: []string{
			ops,
			topic.InfoSubjectWildcard,
			notifyPrefix + s.Persona,
			inboxWildcard,
		},
	}
}

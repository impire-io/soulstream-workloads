// Package runner is the "runner" persona that soulstream's work extension
// anticipates (work.md stage 4): it opens and claims an execution work item,
// launches the workload through a backend, and marks the item done or abandoned
// on exit — so a workload's whole life is ordinary work ops on the topic, the
// single control plane (constitution V).
package runner

import "github.com/impire-io/soulrealm/backend"

// Terminal is the terminal work op for a finished workload.
type Terminal int

const (
	// TerminalDone maps to work.done (clean exit).
	TerminalDone Terminal = iota
	// TerminalAbandon maps to work.abandon (failure/kill).
	TerminalAbandon
)

// Abandon reasons (payload of work.abandon).
const (
	ReasonStartFailed = "start-failed"
	ReasonSignal      = "signal"
	ReasonNonzeroExit = "nonzero-exit"
)

// Outcome classifies how a workload finished into its terminal work op and, for
// an abandon, a reason. startErr is non-nil when the backend failed to start the
// workload at all. Pure: no I/O, unit-tested with no server.
func Outcome(startErr error, st backend.ExitStatus) (Terminal, string) {
	switch {
	case startErr != nil:
		return TerminalAbandon, ReasonStartFailed
	case st.Signalled():
		return TerminalAbandon, ReasonSignal
	case st.Code != 0:
		return TerminalAbandon, ReasonNonzeroExit
	default:
		return TerminalDone, ""
	}
}

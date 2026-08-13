// Package backend is the isolation seam (constitution III): how a workload is
// run is chosen here, never in the declaration. A backend launches a workload
// with an injected credential and reports its exit; it emits no soulstream ops —
// lifecycle publication is the runner's job — so every backend is observable the
// same way (constitution V) and none can leak a private control channel.
package backend

import (
	"context"

	"github.com/impire-io/soulstream-workloads/minter"
)

// ExitStatus describes how a workload process ended. Exactly one of Code/Signal
// is meaningful: a signalled process has a non-empty Signal.
type ExitStatus struct {
	Code   int
	Signal string
}

// Signalled reports whether the workload was killed by a signal.
func (e ExitStatus) Signalled() bool { return e.Signal != "" }

// LaunchSpec is everything a backend needs to run one workload.
type LaunchSpec struct {
	Artifact   string                         // resolved local path (file:// for native, M1.1)
	Args       []string                       // argv passed to the artifact
	Cred       minter.PersonaScopedCredential // the workload's scoped NATS identity
	Realm      string                         // the realm the workload participates in
	Topic      string                         // the topic path the workload participates in
	ScratchDir string                         // private working dir; reaped on exit
}

// Handle is a running workload the runner can observe and stop.
type Handle interface {
	// Wait blocks until the workload exits, reaps its scratch, and returns the
	// exit status. Safe to call more than once (returns the same status).
	Wait() ExitStatus
	// Stop asks the workload to terminate, escalating if ctx expires.
	Stop(ctx context.Context) error
}

// Backend launches a workload artifact with an injected credential.
type Backend interface {
	Start(ctx context.Context, spec LaunchSpec) (Handle, error)
}

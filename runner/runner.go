package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/impire-io/soulstream-workloads/backend"
	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/minter"
)

// TopicClient is the subset of the soulstream topic Handle the runner needs to
// express a workload's lifecycle as work ops. *topic.Handle satisfies it
// structurally, so the runner can be tested with a fake and needs no server.
type TopicClient interface {
	OpenWork(ctx context.Context, title, body string) (string, error)
	ClaimWork(ctx context.Context, itemID string) (string, error)
	CompleteWork(ctx context.Context, itemID string) (string, error)
	AbandonWork(ctx context.Context, itemID string) (string, error)
}

// ArtifactSource resolves a declaration's artifact to the local path a
// backend launches. The shipped occupant is the artifact package's Resolver
// (file:// = the host path; soulstream:// = the record lineage's tip,
// digest-checked, materialised into the run's scratch — reaped with it, never
// a durable copy). The runner stays pure: it never touches NATS itself.
type ArtifactSource interface {
	Resolve(ctx context.Context, d declaration.Declaration, scratchDir string) (string, error)
}

// Runner launches workloads and records their life as work ops. It holds no
// connection itself; the caller injects the topic client for the runner
// persona, keeping the runner testable and the connection lifecycle external.
type Runner struct {
	Minter      minter.Minter
	Backend     backend.Backend
	Realm       string
	CredTTL     time.Duration
	ScratchRoot string
	// Artifacts resolves record-form (soulstream://) artifacts. nil keeps
	// today's file://-only behavior; a record-form declaration then refuses
	// before any op publishes.
	Artifacts ArtifactSource
}

// Running is a launched workload the caller observes and ends. How it ends
// depends on the workload: a self-exiting agent/job is awaited (Wait); a
// persistent service is stopped (Stop). Serve handles either for the CLI.
type Running struct {
	handle backend.Handle
	tc     TopicClient
	itemID string
	base   context.Context // publishes terminal ops even if the launch ctx is cancelled
}

// Launch validates, opens + claims the execution work item, mints the
// workload's scoped credential, and starts it. Preflight failures publish
// nothing (FR-008); a start failure yields work.open + work.abandon(start-failed)
// with no dangling claim. On success it returns a Running handle whose end the
// caller decides.
func (r *Runner) Launch(ctx context.Context, tc TopicClient, d declaration.Declaration) (*Running, error) {
	if err := d.Validate(); err != nil {
		return nil, fmt.Errorf("runner: invalid declaration: %w", err)
	}
	// Preflight (publishes nothing, FR-008): a file:// artifact resolves to
	// its host path right here; a record-form artifact only checks that a
	// source exists — the fetch itself lands in the run's scratch below,
	// after the work item exists to carry a failure.
	ref, err := d.ArtifactRef()
	if err != nil {
		return nil, fmt.Errorf("runner: %w", err)
	}
	artifactPath := ref.Path
	if ref.Scheme != declaration.SchemeFile && r.Artifacts == nil {
		return nil, fmt.Errorf("runner: artifact %s is record-form and no artifact source is configured", d.Artifact)
	}
	cred, err := r.Minter.Mint(minter.Scope{Role: d.Role, Persona: d.Persona, Topic: d.Topic, Capabilities: d.Capabilities}, r.CredTTL)
	if err != nil {
		return nil, fmt.Errorf("runner: mint credential for %q: %w", d.Persona, err)
	}

	title := fmt.Sprintf("run %s as %s", d.Artifact, d.Persona)
	body := fmt.Sprintf("role=%s lifecycle=%s persona=%s topic=%s artifact=%s",
		d.Role, d.Lifecycle, d.Persona, d.Topic, d.Artifact)
	itemID, err := tc.OpenWork(ctx, title, body)
	if err != nil {
		return nil, fmt.Errorf("runner: open work: %w", err)
	}

	scratchDir := filepath.Join(r.ScratchRoot, itemID)
	if artifactPath == "" {
		// The record form: materialise the lineage tip into the run's
		// scratch (digest-checked, reaped with the run). A failure ends the
		// item like a start failure — work.open + work.abandon, no dangling
		// claim.
		resolved, rerr := r.Artifacts.Resolve(ctx, d, scratchDir)
		if rerr != nil {
			if _, aerr := tc.AbandonWork(ctx, itemID); aerr != nil {
				return nil, fmt.Errorf("runner: resolve artifact failed (%v) and abandon failed: %w", rerr, aerr)
			}
			return nil, fmt.Errorf("runner: resolve artifact: %w", rerr)
		}
		artifactPath = resolved
	}

	spec := backend.LaunchSpec{
		Artifact:   artifactPath,
		Args:       d.Args,
		Cred:       cred,
		Realm:      r.Realm,
		Topic:      d.Topic,
		ScratchDir: scratchDir,
	}
	h, startErr := r.Backend.Start(ctx, spec)
	if startErr != nil {
		_, reason := Outcome(startErr, backend.ExitStatus{})
		if _, aerr := tc.AbandonWork(ctx, itemID); aerr != nil {
			return nil, fmt.Errorf("runner: start failed (%v) and abandon failed: %w", startErr, aerr)
		}
		return nil, fmt.Errorf("runner: start workload (%s): %w", reason, startErr)
	}

	if _, err := tc.ClaimWork(ctx, itemID); err != nil {
		return nil, fmt.Errorf("runner: claim work: %w", err)
	}

	return &Running{handle: h, tc: tc, itemID: itemID, base: context.WithoutCancel(ctx)}, nil
}

// Wait blocks until the workload exits on its own and publishes the terminal op
// (work.done for a clean exit, work.abandon otherwise). For self-exiting agents
// and jobs.
func (rw *Running) Wait() error {
	term, _ := Outcome(nil, rw.handle.Wait())
	return rw.terminal(term)
}

// Stop asks the workload to terminate, reaps it, and records work.done —
// stopping a service is an intentional, successful end. For persistent services.
func (rw *Running) Stop(ctx context.Context) error {
	if err := rw.handle.Stop(ctx); err != nil {
		return fmt.Errorf("runner: stop workload: %w", err)
	}
	rw.handle.Wait()
	return rw.terminal(TerminalDone)
}

// Serve blocks until the workload exits on its own (→ terminal op) or ctx is
// cancelled (→ Stop → work.done). The CLI helper for a persistent workload.
func (rw *Running) Serve(ctx context.Context) error {
	exited := make(chan backend.ExitStatus, 1)
	go func() { exited <- rw.handle.Wait() }()
	select {
	case st := <-exited:
		term, _ := Outcome(nil, st)
		return rw.terminal(term)
	case <-ctx.Done():
		_ = rw.handle.Stop(context.Background())
		<-exited
		return rw.terminal(TerminalDone)
	}
}

func (rw *Running) terminal(term Terminal) error {
	switch term {
	case TerminalDone:
		if _, err := rw.tc.CompleteWork(rw.base, rw.itemID); err != nil {
			return fmt.Errorf("runner: complete work: %w", err)
		}
	case TerminalAbandon:
		if _, err := rw.tc.AbandonWork(rw.base, rw.itemID); err != nil {
			return fmt.Errorf("runner: abandon work: %w", err)
		}
	}
	return nil
}

// Run launches a workload and awaits its completion — the M1.1 behaviour, for a
// self-exiting agent or job.
func (r *Runner) Run(ctx context.Context, tc TopicClient, d declaration.Declaration) error {
	rw, err := r.Launch(ctx, tc, d)
	if err != nil {
		return err
	}
	return rw.Wait()
}

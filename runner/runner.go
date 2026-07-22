package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/impire-io/soulrealm/backend"
	"github.com/impire-io/soulrealm/declaration"
	"github.com/impire-io/soulrealm/minter"
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

// Runner launches one workload and records its life as work ops. It holds no
// connection itself; the caller injects the topic client for the runner
// persona, keeping the runner testable and the connection lifecycle external.
type Runner struct {
	Minter      minter.Minter
	Backend     backend.Backend
	Realm       string
	CredTTL     time.Duration
	ScratchRoot string
}

// Run executes one declaration to completion:
//
//	preflight (validate, resolve artifact, mint) → work.open → backend.Start
//	→ work.claim → wait → work.done | work.abandon
//
// Preflight failures publish nothing (FR-008: no silent partial start); a
// start failure yields work.open + work.abandon(start-failed) with no dangling
// claim; the terminal op is exactly one of done/abandon.
func (r *Runner) Run(ctx context.Context, tc TopicClient, d declaration.Declaration) error {
	// Preflight — nothing is published until these pass.
	if err := d.Validate(); err != nil {
		return fmt.Errorf("runner: invalid declaration: %w", err)
	}
	artifactPath, err := d.ArtifactPath()
	if err != nil {
		return fmt.Errorf("runner: %w", err)
	}
	cred, err := r.Minter.Mint(minter.Scope{Persona: d.Persona, Topic: d.Topic}, r.CredTTL)
	if err != nil {
		return fmt.Errorf("runner: mint credential for %q: %w", d.Persona, err)
	}

	// requested
	title := fmt.Sprintf("run %s as %s", d.Artifact, d.Persona)
	body := fmt.Sprintf("role=%s lifecycle=%s persona=%s topic=%s artifact=%s",
		d.Role, d.Lifecycle, d.Persona, d.Topic, d.Artifact)
	itemID, err := tc.OpenWork(ctx, title, body)
	if err != nil {
		return fmt.Errorf("runner: open work: %w", err)
	}

	// launch
	spec := backend.LaunchSpec{
		Artifact:   artifactPath,
		Args:       d.Args,
		Cred:       cred,
		Realm:      r.Realm,
		Topic:      d.Topic,
		ScratchDir: filepath.Join(r.ScratchRoot, itemID),
	}
	h, startErr := r.Backend.Start(ctx, spec)
	if startErr != nil {
		_, reason := Outcome(startErr, backend.ExitStatus{})
		if _, aerr := tc.AbandonWork(ctx, itemID); aerr != nil {
			return fmt.Errorf("runner: start failed (%v) and abandon failed: %w", startErr, aerr)
		}
		return fmt.Errorf("runner: start workload (%s): %w", reason, startErr)
	}

	// started
	if _, err := tc.ClaimWork(ctx, itemID); err != nil {
		return fmt.Errorf("runner: claim work: %w", err)
	}

	// exited → terminal op
	st := h.Wait()
	term, _ := Outcome(nil, st)
	switch term {
	case TerminalDone:
		if _, err := tc.CompleteWork(ctx, itemID); err != nil {
			return fmt.Errorf("runner: complete work: %w", err)
		}
	case TerminalAbandon:
		if _, err := tc.AbandonWork(ctx, itemID); err != nil {
			return fmt.Errorf("runner: abandon work: %w", err)
		}
	}
	return nil
}

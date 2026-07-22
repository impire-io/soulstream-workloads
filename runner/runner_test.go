package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/impire-io/soulrealm/backend"
	"github.com/impire-io/soulrealm/declaration"
	"github.com/impire-io/soulrealm/minter"
)

type fakeTopic struct {
	calls   []string
	openErr error
}

func (f *fakeTopic) OpenWork(_ context.Context, _, _ string) (string, error) {
	f.calls = append(f.calls, "open")
	return "item-1", f.openErr
}
func (f *fakeTopic) ClaimWork(_ context.Context, _ string) (string, error) {
	f.calls = append(f.calls, "claim")
	return "op", nil
}
func (f *fakeTopic) CompleteWork(_ context.Context, _ string) (string, error) {
	f.calls = append(f.calls, "done")
	return "op", nil
}
func (f *fakeTopic) AbandonWork(_ context.Context, _ string) (string, error) {
	f.calls = append(f.calls, "abandon")
	return "op", nil
}

type fakeBackend struct {
	startErr error
	status   backend.ExitStatus
}

func (b fakeBackend) Start(_ context.Context, _ backend.LaunchSpec) (backend.Handle, error) {
	if b.startErr != nil {
		return nil, b.startErr
	}
	return fakeHandle{b.status}, nil
}

type fakeHandle struct{ st backend.ExitStatus }

func (h fakeHandle) Wait() backend.ExitStatus     { return h.st }
func (h fakeHandle) Stop(_ context.Context) error { return nil }

type fakeMinter struct{ err error }

func (m fakeMinter) Mint(s minter.Scope, _ time.Duration) (minter.PersonaScopedCredential, error) {
	if m.err != nil {
		return minter.PersonaScopedCredential{}, m.err
	}
	return minter.PersonaScopedCredential{Persona: s.Persona, NatsServers: []string{"nats://x"}}, nil
}

func validDecl() declaration.Declaration {
	return declaration.Declaration{
		Role:      declaration.RoleAgent,
		Lifecycle: declaration.LifecycleService,
		Persona:   "researcher",
		Topic:     "t-ab12",
		Artifact:  "file:///bin/true",
	}
}

func newRunner(m minter.Minter, b backend.Backend) *Runner {
	return &Runner{Minter: m, Backend: b, Realm: "acme", CredTTL: time.Hour, ScratchRoot: "/tmp/sr"}
}

func TestRunCleanExitOpsSequence(t *testing.T) {
	tc := &fakeTopic{}
	r := newRunner(fakeMinter{}, fakeBackend{status: backend.ExitStatus{Code: 0}})
	if err := r.Run(context.Background(), tc, validDecl()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.Join(tc.calls, ","); got != "open,claim,done" {
		t.Fatalf("ops = %q, want open,claim,done", got)
	}
}

func TestRunNonzeroExitAbandons(t *testing.T) {
	tc := &fakeTopic{}
	r := newRunner(fakeMinter{}, fakeBackend{status: backend.ExitStatus{Code: 7}})
	if err := r.Run(context.Background(), tc, validDecl()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.Join(tc.calls, ","); got != "open,claim,abandon" {
		t.Fatalf("ops = %q, want open,claim,abandon", got)
	}
}

// FR-008: a start failure yields open + abandon, with no dangling claim.
func TestRunStartFailureAbandonsNoClaim(t *testing.T) {
	tc := &fakeTopic{}
	r := newRunner(fakeMinter{}, fakeBackend{startErr: errors.New("no such file")})
	err := r.Run(context.Background(), tc, validDecl())
	if err == nil {
		t.Fatal("expected an error on start failure")
	}
	if got := strings.Join(tc.calls, ","); got != "open,abandon" {
		t.Fatalf("ops = %q, want open,abandon (no claim)", got)
	}
}

// FR-008: a preflight failure (mint) publishes nothing at all.
func TestRunMintFailurePublishesNothing(t *testing.T) {
	tc := &fakeTopic{}
	r := newRunner(fakeMinter{err: errors.New("no signing key")}, fakeBackend{})
	err := r.Run(context.Background(), tc, validDecl())
	if err == nil {
		t.Fatal("expected an error on mint failure")
	}
	if len(tc.calls) != 0 {
		t.Fatalf("ops = %v, want none (no partial start)", tc.calls)
	}
}

// A persistent service is launched (open+claim), stays up, and Stop records a
// clean done — even though the process exits via signal (intentional stop).
func TestLaunchThenStopCompletes(t *testing.T) {
	tc := &fakeTopic{}
	r := newRunner(fakeMinter{}, fakeBackend{status: backend.ExitStatus{Signal: "terminated"}})
	rw, err := r.Launch(context.Background(), tc, validDecl())
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if got := strings.Join(tc.calls, ","); got != "open,claim" {
		t.Fatalf("after launch ops = %q, want open,claim", got)
	}
	if err := rw.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := strings.Join(tc.calls, ","); got != "open,claim,done" {
		t.Fatalf("after stop ops = %q, want open,claim,done", got)
	}
}

func TestRunInvalidDeclarationRefused(t *testing.T) {
	tc := &fakeTopic{}
	r := newRunner(fakeMinter{}, fakeBackend{})
	d := validDecl()
	d.Lifecycle = declaration.LifecycleFunction // still deferred
	if err := r.Run(context.Background(), tc, d); err == nil {
		t.Fatal("expected invalid declaration to be refused")
	}
	if len(tc.calls) != 0 {
		t.Fatalf("ops = %v, want none", tc.calls)
	}
}

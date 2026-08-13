// Package native runs workloads as local OS processes — the first (reference)
// isolation backend. It injects the workload's scoped credential through a
// deliberately CLEAN environment: soulstream-workloads's own process env (which may hold
// the realm signing key) is NOT inherited by the child, so a workload sees only
// its own scoped identity (constitution II).
package native

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/jwt/v2"

	"github.com/impire-io/soulstream-workloads/backend"
)

// Environment variables the workload receives.
const (
	EnvNatsServers = "SOULSTREAM_NATS_SERVERS"
	EnvCredsFile   = "SOULSTREAM_NATS_CREDS"
	EnvRealm       = "SOULSTREAM_REALM"
	EnvPersona     = "SOULSTREAM_PERSONA"
	EnvTopic       = "SOULSTREAM_TOPIC"
)

const stopGrace = 5 * time.Second

// Backend launches workloads as native processes.
type Backend struct{}

// New returns a native backend.
func New() *Backend { return &Backend{} }

// Start writes the workload's creds to its scratch dir, builds a clean env, and
// starts the process. It does not block.
func (b *Backend) Start(_ context.Context, spec backend.LaunchSpec) (backend.Handle, error) {
	if err := os.MkdirAll(spec.ScratchDir, 0o700); err != nil {
		return nil, fmt.Errorf("native: scratch dir: %w", err)
	}

	credsPath := filepath.Join(spec.ScratchDir, "nats.creds")
	credsBody, err := jwt.FormatUserConfig(spec.Cred.UserJWT, spec.Cred.UserSeed)
	if err != nil {
		_ = os.RemoveAll(spec.ScratchDir)
		return nil, fmt.Errorf("native: format creds: %w", err)
	}
	if err := os.WriteFile(credsPath, credsBody, 0o600); err != nil {
		_ = os.RemoveAll(spec.ScratchDir)
		return nil, fmt.Errorf("native: write creds: %w", err)
	}

	cmd := exec.Command(spec.Artifact, spec.Args...)
	cmd.Dir = spec.ScratchDir
	cmd.Env = cleanEnv(spec, credsPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(spec.ScratchDir)
		return nil, fmt.Errorf("native: start %q: %w", spec.Artifact, err)
	}

	return &handle{cmd: cmd, scratchDir: spec.ScratchDir}, nil
}

// cleanEnv builds the child's environment from scratch. It carries only what a
// workload needs — PATH/HOME for basic operation plus its scoped identity —
// and deliberately excludes soulstream-workloads's own process env so no soulstream-workloads secret
// (e.g. the realm signing key) can leak into a workload.
func cleanEnv(spec backend.LaunchSpec, credsPath string) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		EnvNatsServers + "=" + strings.Join(spec.Cred.NatsServers, ","),
		EnvCredsFile + "=" + credsPath,
		EnvRealm + "=" + spec.Realm,
		EnvPersona + "=" + spec.Cred.Persona,
		EnvTopic + "=" + spec.Topic,
	}
}

type handle struct {
	cmd        *exec.Cmd
	scratchDir string

	once   sync.Once
	status backend.ExitStatus
}

// Wait blocks for the process, reaps the scratch dir, and returns the status.
func (h *handle) Wait() backend.ExitStatus {
	h.once.Do(func() {
		h.status = statusOf(h.cmd.Wait())
		_ = os.RemoveAll(h.scratchDir)
	})
	return h.status
}

// Stop sends SIGTERM, then SIGKILL if ctx expires or the grace elapses.
func (h *handle) Stop(ctx context.Context) error {
	if h.cmd.Process == nil {
		return nil
	}
	if err := h.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("native: signal: %w", err)
	}

	deadline := time.NewTimer(stopGrace)
	defer deadline.Stop()
	select {
	case <-ctx.Done():
	case <-deadline.C:
	}
	// If still running, escalate. Kill is idempotent enough for our purpose.
	_ = h.cmd.Process.Signal(syscall.SIGKILL)
	return nil
}

// statusOf maps the result of exec.Cmd.Wait into an ExitStatus.
func statusOf(err error) backend.ExitStatus {
	if err == nil {
		return backend.ExitStatus{Code: 0}
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() {
				return backend.ExitStatus{Signal: ws.Signal().String()}
			}
			return backend.ExitStatus{Code: ws.ExitStatus()}
		}
		return backend.ExitStatus{Code: ee.ExitCode()}
	}
	// Non-exit error (e.g. wait failed); treat as a non-zero, uncoded failure.
	return backend.ExitStatus{Code: -1}
}

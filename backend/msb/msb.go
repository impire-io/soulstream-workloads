// Package msb runs workloads inside microsandbox microVMs — one sandbox per
// workload, the second isolation backend behind the seam (constitution III).
// It supervises the `msb` CLI as its child process exactly the way the native
// backend supervises the workload itself: the guest command's exit code
// propagates through `msb run` (measured, research D1), and terminating the
// child stops the VM (D4). The workload sees the same SOULREALM_* env
// contract as under native — only the values adapt to the guest: the creds
// file appears at an in-guest path and loopback NATS URLs are rewritten to
// the host alias, reachable under the sandbox's `host`-only network policy
// (D2). Like every backend it publishes no ops and owns no control channel;
// lifecycle belongs to the runner (constitutions I and V).
package msb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/jwt/v2"

	"github.com/impire-io/soulrealm/backend"
	"github.com/impire-io/soulrealm/backend/native"
)

// Defaults for the node-side backend configuration. None of these may appear
// in a declaration — they are how THIS node isolates, never what a workload is.
const (
	DefaultImage     = "alpine"
	DefaultMsbPath   = "msb"
	DefaultHostAlias = "host.microsandbox.internal"
)

// In-guest layout: the scratch dir is bind-mounted read-write and is the
// workload's working directory (native parity); the artifact is copied into
// the guest rootfs before boot, so the host copy is never exposed to the VM.
const (
	guestScratch     = "/scratch"
	guestCredsPath   = guestScratch + "/nats.creds"
	guestArtifactDir = "/artifact"
	sandboxPrefix    = "soulrealm-"
)

// stopGrace mirrors the native backend's SIGTERM→SIGKILL grace. A var so
// tests can shorten it.
var stopGrace = 5 * time.Second

// Backend launches workloads in microsandbox microVMs via the `msb` CLI.
// The zero value works; fields override node-side choices only.
type Backend struct {
	// Image is the OCI image booted as the guest (any image able to exec a
	// static binary suffices).
	Image string
	// MsbPath is the msb executable to invoke — also the unit-test seam.
	MsbPath string
	// HostAlias is the in-guest name for the host's loopback, which loopback
	// NATS URLs are rewritten to.
	HostAlias string
}

// New returns an msb backend with the default node-side configuration.
func New() *Backend { return &Backend{} }

func (b *Backend) image() string {
	if b.Image != "" {
		return b.Image
	}
	return DefaultImage
}

func (b *Backend) msbPath() string {
	if b.MsbPath != "" {
		return b.MsbPath
	}
	return DefaultMsbPath
}

func (b *Backend) hostAlias() string {
	if b.HostAlias != "" {
		return b.HostAlias
	}
	return DefaultHostAlias
}

// Start writes the workload's creds into its scratch dir (native parity),
// then boots one named sandbox around the artifact. It does not block.
func (b *Backend) Start(_ context.Context, spec backend.LaunchSpec) (backend.Handle, error) {
	if err := os.MkdirAll(spec.ScratchDir, 0o700); err != nil {
		return nil, fmt.Errorf("msb: scratch dir: %w", err)
	}

	// msb 0.6.7 fails to mount a source whose path traverses a symlink
	// (measured: "Not a directory" — macOS tempdirs sit behind /var →
	// /private/var), so both node-side paths are resolved before handover.
	// The sandbox name is derived first: it is the work-item id either way.
	name := sandboxName(spec.ScratchDir)
	scratch, err := filepath.EvalSymlinks(spec.ScratchDir)
	if err != nil {
		_ = os.RemoveAll(spec.ScratchDir)
		return nil, fmt.Errorf("msb: resolve scratch dir: %w", err)
	}
	spec.ScratchDir = scratch
	artifact, err := filepath.EvalSymlinks(spec.Artifact)
	if err != nil {
		_ = os.RemoveAll(spec.ScratchDir)
		return nil, fmt.Errorf("msb: resolve artifact: %w", err)
	}
	spec.Artifact = artifact

	credsBody, err := jwt.FormatUserConfig(spec.Cred.UserJWT, spec.Cred.UserSeed)
	if err != nil {
		_ = os.RemoveAll(spec.ScratchDir)
		return nil, fmt.Errorf("msb: format creds: %w", err)
	}
	if err := os.WriteFile(filepath.Join(spec.ScratchDir, "nats.creds"), credsBody, 0o600); err != nil {
		_ = os.RemoveAll(spec.ScratchDir)
		return nil, fmt.Errorf("msb: write creds: %w", err)
	}

	cmd := exec.Command(b.msbPath(), b.runArgs(name, spec)...)
	cmd.Env = hostEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(spec.ScratchDir)
		return nil, fmt.Errorf("msb: start %q: %w", spec.Artifact, err)
	}

	return &handle{cmd: cmd, scratchDir: spec.ScratchDir, name: name, msbPath: b.msbPath()}, nil
}

// runArgs builds the `msb run` invocation for one workload: a named sandbox,
// scratch bind-mounted read-write as the workdir, the artifact copied into
// the guest rootfs pre-boot (`--copy-file` — the `:ro` mount option is
// rejected by msb 0.6.7, and a copy exposes no host path at all), the
// deny-by-default network policy opened only toward the host, and the
// workload env contract passed with -e.
func (b *Backend) runArgs(name string, spec backend.LaunchSpec) []string {
	guestArtifact := guestArtifactDir + "/" + filepath.Base(spec.Artifact)
	args := []string{
		"run", "--no-tty", "--quiet",
		"--name", name,
		"-v", spec.ScratchDir + ":" + guestScratch,
		"--copy-file", spec.Artifact + ":" + guestArtifact,
		"-w", guestScratch,
		"--net", "host",
	}
	for _, kv := range guestEnv(spec, b.hostAlias()) {
		args = append(args, "-e", kv)
	}
	args = append(args, b.image(), "--", guestArtifact)
	return append(args, spec.Args...)
}

// guestEnv is the workload-env contract — the same variable names and
// semantics as the native backend, with values adapted to the guest.
func guestEnv(spec backend.LaunchSpec, alias string) []string {
	return []string{
		native.EnvNatsServers + "=" + strings.Join(rewriteServers(spec.Cred.NatsServers, alias), ","),
		native.EnvCredsFile + "=" + guestCredsPath,
		native.EnvRealm + "=" + spec.Realm,
		native.EnvPersona + "=" + spec.Cred.Persona,
		native.EnvTopic + "=" + spec.Topic,
	}
}

// hostEnv is the environment for the msb process itself: enough to operate
// (PATH, HOME for ~/.microsandbox) and nothing of soulrealm's own env, so no
// soulrealm secret can leak toward the sandbox machinery (constitution II).
func hostEnv() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
	}
}

// rewriteServers maps loopback hosts in NATS URLs to the in-guest host alias.
// Only loopback is rewritten: from inside the VM, 127.0.0.1 is the guest
// itself, and the alias is microsandbox's name for the host's loopback. Other
// hosts pass through untouched (a non-loopback server would additionally need
// the `public` network profile — a named limitation until Fleet).
func rewriteServers(servers []string, alias string) []string {
	out := make([]string, len(servers))
	for i, s := range servers {
		out[i] = rewriteServer(s, alias)
	}
	return out
}

func rewriteServer(server, alias string) string {
	if u, err := url.Parse(server); err == nil && u.Host != "" {
		if !isLoopback(u.Hostname()) {
			return server
		}
		if p := u.Port(); p != "" {
			u.Host = alias + ":" + p
		} else {
			u.Host = alias
		}
		return u.String()
	}
	// Bare host[:port] forms.
	if host, port, err := net.SplitHostPort(server); err == nil {
		if isLoopback(host) {
			return net.JoinHostPort(alias, port)
		}
		return server
	}
	if isLoopback(server) {
		return alias
	}
	return server
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// sandboxName derives the sandbox's name from the scratch dir's base — the
// work item id — so a topic op, a scratch dir, and a `msb ls` row all name
// the same run. Sanitized conservatively; msb caps names at 128 bytes.
func sandboxName(scratchDir string) string {
	base := filepath.Base(scratchDir)
	mapped := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, base)
	const maxBase = 100
	if len(mapped) > maxBase {
		mapped = mapped[:maxBase]
	}
	return sandboxPrefix + mapped
}

type handle struct {
	cmd        *exec.Cmd
	scratchDir string
	name       string
	msbPath    string

	once   sync.Once
	status backend.ExitStatus
}

// Wait blocks for the msb process (its exit status is the guest command's —
// measured), then reaps everything the launch created: the sandbox record
// (`msb rm --force`) and the scratch dir. Idempotent.
func (h *handle) Wait() backend.ExitStatus {
	h.once.Do(func() {
		h.status = statusOf(h.cmd.Wait())
		rm := exec.Command(h.msbPath, "rm", "--force", h.name)
		rm.Env = hostEnv()
		_ = rm.Run()
		_ = os.RemoveAll(h.scratchDir)
	})
	return h.status
}

// Stop sends SIGTERM to the msb process — which stops the VM (measured) —
// then SIGKILL if ctx expires or the grace elapses. Native semantics.
func (h *handle) Stop(ctx context.Context) error {
	if h.cmd.Process == nil {
		return nil
	}
	if err := h.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("msb: signal: %w", err)
	}

	deadline := time.NewTimer(stopGrace)
	defer deadline.Stop()
	select {
	case <-ctx.Done():
	case <-deadline.C:
	}
	_ = h.cmd.Process.Signal(syscall.SIGKILL)
	return nil
}

// statusOf maps the result of exec.Cmd.Wait into an ExitStatus (the same
// mapping as the native backend — the seam's exit-fidelity requirement).
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
	return backend.ExitStatus{Code: -1}
}

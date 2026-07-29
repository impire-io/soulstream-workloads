//go:build msb_e2e || k8s_e2e

// Helpers shared by the msb_e2e and k8s_e2e suites — tag-gated with them so
// the default (hermetic) build carries no unused code.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nats-io/nkeys"

	"github.com/impire-io/soulrealm/minter"
)

// buildCmdLinux builds a command for an isolated guest/pod: linux on the
// host's arch (the guest always matches the host's GOARCH — M1.3 research
// D5), static (CGO_ENABLED=0) so any minimal image can exec it.
func buildCmdLinux(t *testing.T, importPath string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), filepath.Base(importPath)+"-linux")
	cmd := exec.Command("go", "build", "-o", bin, importPath)
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build linux %s: %v\n%s", importPath, err, out)
	}
	return bin
}

// buildInline builds a one-file Go program (host or guest arch) so probe
// workloads need no cmd/ package.
func buildInline(t *testing.T, name, src string, linux bool) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+name+"\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	bin := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = dir
	if linux {
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, out)
	}
	return bin
}

// e2eMinter returns a throwaway-account minter (an open in-process server
// does not enforce scope — that is the operator-mode tests' ground; here the
// mint+delivery path is what is exercised).
func e2eMinter(t *testing.T, url string) *minter.SigningKeyMinter {
	t.Helper()
	acc, _ := nkeys.CreateAccount()
	pub, _ := acc.PublicKey()
	seed, _ := acc.Seed()
	m, err := minter.NewSigningKeyMinter(seed, pub, []string{url})
	if err != nil {
		t.Fatalf("minter: %v", err)
	}
	return m
}

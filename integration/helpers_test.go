package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nats-io/jwt/v2"

	"github.com/impire-io/soulrealm/minter"
)

// buildCmd builds a command from the module by import path and returns the
// binary path.
func buildCmd(t *testing.T, importPath string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), filepath.Base(importPath))
	cmd := exec.Command("go", "build", "-o", bin, importPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", importPath, err, out)
	}
	return bin
}

// writeCreds writes a minted credential to a NATS creds file and returns its path.
func writeCreds(t *testing.T, cred minter.PersonaScopedCredential) string {
	t.Helper()
	body, err := jwt.FormatUserConfig(cred.UserJWT, cred.UserSeed)
	if err != nil {
		t.Fatalf("format creds: %v", err)
	}
	path := filepath.Join(t.TempDir(), "u.creds")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	return path
}

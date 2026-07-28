// Command soulrealm launches workloads onto a node. M1.1: a single
// `soulrealm workload start <declaration-file>` that runs one agent.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/impire-io/soulstream/realm"
	"github.com/impire-io/soulstream/topic"

	"github.com/impire-io/soulrealm/backend"
	"github.com/impire-io/soulrealm/backend/msb"
	"github.com/impire-io/soulrealm/backend/native"
	"github.com/impire-io/soulrealm/declaration"
	"github.com/impire-io/soulrealm/minter"
	"github.com/impire-io/soulrealm/runner"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "soulrealm:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 3 || args[0] != "workload" || args[1] != "start" {
		return fmt.Errorf("usage: soulrealm workload start <declaration-file>")
	}

	data, err := os.ReadFile(args[2])
	if err != nil {
		return fmt.Errorf("read declaration: %w", err)
	}
	d, err := declaration.Parse(data)
	if err != nil {
		return err
	}
	if err := d.Validate(); err != nil {
		return err
	}

	realmName := os.Getenv("SOULREALM_REALM")
	persona := os.Getenv("SOULREALM_PERSONA")
	signingSeed := os.Getenv("SOULREALM_REALM_SIGNING_KEY")
	rootAccount := os.Getenv("SOULREALM_ROOT_ACCOUNT")
	if realmName == "" || persona == "" || signingSeed == "" || rootAccount == "" {
		return fmt.Errorf("SOULREALM_REALM, SOULREALM_PERSONA, SOULREALM_REALM_SIGNING_KEY and SOULREALM_ROOT_ACCOUNT are all required")
	}

	ctx := context.Background()
	client, err := realm.Connect(ctx, realm.Config{
		ContextName: os.Getenv("SOULREALM_CONTEXT"),
		Realm:       realmName,
		Persona:     persona,
	})
	if err != nil {
		return fmt.Errorf("connect to realm: %w", err)
	}
	defer func() { _ = client.Close() }()

	m, err := minter.NewSigningKeyMinter([]byte(signingSeed), rootAccount, serversOf(client))
	if err != nil {
		return err
	}

	be, err := selectBackend(os.Getenv("SOULREALM_BACKEND"), os.Getenv("SOULREALM_MSB_IMAGE"))
	if err != nil {
		return err
	}

	r := &runner.Runner{
		Minter:      m,
		Backend:     be,
		Realm:       realmName,
		CredTTL:     24 * time.Hour,
		ScratchRoot: scratchRoot(),
	}

	rw, err := r.Launch(ctx, topic.Open(client, d.Topic), d)
	if err != nil {
		return err
	}

	// Serve until the workload exits on its own (agent/job) or we are signalled
	// to stop it (a persistent service).
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return rw.Serve(sigCtx)
}

// selectBackend maps the node-side backend choice (SOULREALM_BACKEND) to an
// isolation backend — the ONLY place isolation is chosen (constitution III;
// the declaration cannot name a backend, its parser rejects unknown fields).
// An unrecognised value fails loud before any op is published (FR-001).
func selectBackend(name, msbImage string) (backend.Backend, error) {
	switch name {
	case "", "native":
		return native.New(), nil
	case "msb":
		return &msb.Backend{Image: msbImage}, nil
	default:
		return nil, fmt.Errorf("SOULREALM_BACKEND %q is not a known backend (native, msb)", name)
	}
}

// serversOf returns the realm's NATS server URLs, minted into workload creds so
// the workload knows where to connect. Falls back to the connected URL.
func serversOf(c *realm.Client) []string {
	if s := c.Conn().Servers(); len(s) > 0 {
		return s
	}
	if u := c.Conn().ConnectedUrl(); u != "" {
		return []string{u}
	}
	return nil
}

func scratchRoot() string {
	if d := os.Getenv("SOULREALM_SCRATCH"); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "soulrealm")
}

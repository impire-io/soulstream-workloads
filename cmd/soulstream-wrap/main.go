// Command soulstream-wrap runs one agent where you are: it wraps the
// assistant already installed (and logged in) on this machine so mentions of
// the agent's persona become invocations and every wake leaves exactly one
// outcome in the topic. It is the first occupant of the core CLI's external
// subcommand seam — `soulstream wrap …` reaches it with the resolved
// identity in the environment — and it reads the same SOULSTREAM_* names the
// stdio MCP door reads: the agent's credential block is its whole
// configuration.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/impire-io/soulstream-core/realm"

	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/wrap"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "soulstream-wrap:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("soulstream-wrap", flag.ContinueOnError)
	harness := fs.String("harness", "", "preset: claude | codex")
	templateFile := fs.String("template", "", "custom template file (overrides --harness)")
	declFile := fs.String("declaration", "", "agent declaration file — its wake entries drive the engine (mention-only without it)")
	scratch := fs.String("scratch", "", "run-directory root (default: a temp dir)")
	runTimeout := fs.Duration("run-timeout", 150*time.Second, "harness time budget per attempt")
	retries := fs.Int("retries", 2, "harness attempts per wake before the self-report")
	inboxLimit := fs.Int("inbox-limit", 0, "catch-up depth (0 = the default of 50)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Identity and lane arrive as environment — the same names the MCP door
	// reads, the same block the shell hands out; `soulstream wrap` passes the
	// resolved identity down the same way.
	lane := wrap.Lane{
		URL:       os.Getenv("SOULSTREAM_URL"),
		CredsFile: os.Getenv("SOULSTREAM_CREDS"),
		Token:     os.Getenv("SOULSTREAM_TOKEN"),
		Realm:     os.Getenv("SOULSTREAM_REALM"),
		Persona:   os.Getenv("SOULSTREAM_PERSONA"),
	}
	contextName := os.Getenv("SOULSTREAM_CONTEXT")
	if lane.Persona == "" {
		return fmt.Errorf("a persona is required (SOULSTREAM_PERSONA, or `soulstream --persona <name> wrap`)")
	}
	if lane.Realm == "" {
		return fmt.Errorf("a realm is required (SOULSTREAM_REALM, or `soulstream --realm <name> wrap`)")
	}
	if lane.URL == "" && contextName == "" {
		return fmt.Errorf("a connection is required: SOULSTREAM_URL (the credential block) or SOULSTREAM_CONTEXT")
	}

	var tpl wrap.Template
	var err error
	switch {
	case *templateFile != "":
		tpl, err = wrap.LoadTemplate(*templateFile)
	case *harness != "":
		tpl, err = wrap.Preset(*harness, lane)
	default:
		return fmt.Errorf("pick an assistant: --harness claude|codex, or --template <file>")
	}
	if err != nil {
		return err
	}

	root := *scratch
	if root == "" {
		root = filepath.Join(os.TempDir(), "soulstream-wrap", lane.Persona)
	}

	ctx := context.Background()
	// The connection IS the credential check: a revoked agent is refused
	// here, loudly, and nothing is ever posted in its name.
	client, err := realm.Connect(ctx, realm.Config{
		ContextName: contextName,
		URL:         lane.URL,
		CredsFile:   lane.CredsFile,
		Token:       lane.Token,
		Realm:       lane.Realm,
		Persona:     lane.Persona,
	})
	if err != nil {
		return fmt.Errorf("this agent could not get in (revoked, or the realm is unreachable): %w", err)
	}
	defer func() { _ = client.Close() }()

	cfg := wrap.Config{
		Persona:    lane.Persona,
		Template:   tpl,
		Scratch:    root,
		RunTimeout: *runTimeout,
		Retries:    *retries,
		InboxLimit: *inboxLimit,
	}
	if *declFile != "" {
		// Declaration-driven operation: the record's wake vocabulary drives
		// the engine — mention wakes run only if declared, non-record
		// outcomes land on the declared home topic, instructions ride from
		// the record at every wake. The declaration's persona must be the
		// credential's persona: the connection is the authority.
		raw, err := os.ReadFile(*declFile)
		if err != nil {
			return fmt.Errorf("read declaration: %w", err)
		}
		d, err := declaration.Parse(raw)
		if err != nil {
			return err
		}
		cfg, err = wrap.DeclaredConfig(cfg, d, client)
		if err != nil {
			return err
		}
	}
	w := &wrap.Wrapper{
		Config: cfg,
		Client: client,
		Log:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := w.Run(sigCtx); err != nil && err != context.Canceled {
		return err
	}
	return nil
}

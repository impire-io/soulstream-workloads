// Command tool-upper is a minimal soulrealm tool — the reference `tool`
// workload for the M1.2 slice. It serves an uppercasing capability over
// request-reply on its service subject (SOULSTREAM.SVC.<persona>) and runs
// until soulrealm stops it.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nats-io/nats.go"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tool-upper:", err)
		os.Exit(1)
	}
}

func run() error {
	servers := os.Getenv("SOULREALM_NATS_SERVERS")
	creds := os.Getenv("SOULREALM_NATS_CREDS")
	persona := os.Getenv("SOULREALM_PERSONA")
	if servers == "" || persona == "" {
		return fmt.Errorf("missing SOULREALM_NATS_SERVERS/PERSONA in environment")
	}

	var opts []nats.Option
	if creds != "" {
		opts = append(opts, nats.UserCredentials(creds))
	}
	nc, err := nats.Connect(strings.Split(servers, ",")[0], opts...)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer nc.Drain() //nolint:errcheck // best-effort on shutdown

	subject := "SOULREALM.SVC." + persona
	if _, err := nc.Subscribe(subject, func(m *nats.Msg) {
		_ = m.Respond([]byte(strings.ToUpper(string(m.Data))))
	}); err != nil {
		return fmt.Errorf("subscribe %s: %w", subject, err)
	}
	if err := nc.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	fmt.Println("tool-upper: serving", subject)

	// Run until soulrealm stops us (SIGTERM) or Ctrl-C.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	<-ctx.Done()
	return nil
}

// Command agent-echo is a minimal soulstream-workloads agent — the reference workload
// artifact for the M1.1 slice and quickstart. It reads the credentials and
// realm/topic that soulstream-workloads injected into its environment, connects as its
// persona, posts one turn, and exits.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "agent-echo:", err)
		os.Exit(1)
	}
}

func run() error {
	servers := os.Getenv("SOULSTREAM_NATS_SERVERS")
	creds := os.Getenv("SOULSTREAM_NATS_CREDS")
	realmName := os.Getenv("SOULSTREAM_REALM")
	persona := os.Getenv("SOULSTREAM_PERSONA")
	topicPath := os.Getenv("SOULSTREAM_TOPIC")
	if servers == "" || realmName == "" || persona == "" || topicPath == "" {
		return fmt.Errorf("missing SOULSTREAM_NATS_SERVERS/REALM/PERSONA/TOPIC in environment")
	}

	var opts []nats.Option
	if creds != "" {
		opts = append(opts, nats.UserCredentials(creds))
	}
	nc, err := nats.Connect(strings.Split(servers, ",")[0], opts...)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer nc.Close()

	client, err := realm.NewClient(context.Background(), nc, realm.Config{Realm: realmName, Persona: persona})
	if err != nil {
		return fmt.Errorf("realm client: %w", err)
	}
	defer func() { _ = client.Close() }()

	h := topic.Open(client, topicPath)
	if _, err := h.PostTurn(context.Background(), "hello from "+persona); err != nil {
		return fmt.Errorf("post turn: %w", err)
	}
	fmt.Println("agent-echo: posted a turn as", persona)
	return nil
}

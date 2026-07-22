// Command agent-echo is a minimal soulrealm agent — the reference workload
// artifact for the M1.1 slice and quickstart. It reads the credentials and
// realm/topic that soulrealm injected into its environment, connects as its
// persona, posts one turn, and exits.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream/realm"
	"github.com/impire-io/soulstream/topic"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "agent-echo:", err)
		os.Exit(1)
	}
}

func run() error {
	servers := os.Getenv("SOULREALM_NATS_SERVERS")
	creds := os.Getenv("SOULREALM_NATS_CREDS")
	realmName := os.Getenv("SOULREALM_REALM")
	persona := os.Getenv("SOULREALM_PERSONA")
	topicPath := os.Getenv("SOULREALM_TOPIC")
	if servers == "" || realmName == "" || persona == "" || topicPath == "" {
		return fmt.Errorf("missing SOULREALM_NATS_SERVERS/REALM/PERSONA/TOPIC in environment")
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

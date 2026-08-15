// harness-mock is the hermetic stand-in for a headless coding harness: it
// emits the measured event grammars (claude-shaped flat events, codex-shaped
// nested events) so the waker's template mapping is proven against both
// without any real harness or auth on the machine. Its fault modes replay the
// research's fault trials: die mid-run, hang past the budget, or post its own
// reply through the record mid-run (the self-post correlation case).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "harness-mock:", err)
		os.Exit(1)
	}
}

func run() error {
	grammar := flag.String("grammar", "claude", "claude | codex")
	reply := flag.String("reply", "mock reply", "the terminal event's text")
	mode := flag.String("mode", "ok", "ok | die | hang | self-post")
	url := flag.String("url", "", "NATS URL (self-post mode)")
	realmName := flag.String("realm", "", "realm (self-post mode)")
	persona := flag.String("persona", "", "persona (self-post mode)")
	topicPath := flag.String("topic", "", "topic path (self-post mode)")
	flag.Parse()

	narrate(*grammar)

	switch *mode {
	case "ok":
		terminal(*grammar, *reply)
		return nil
	case "die":
		os.Exit(3)
		return nil
	case "hang":
		time.Sleep(300 * time.Second)
		return nil
	case "self-post":
		if err := selfPost(*url, *realmName, *persona, *topicPath, *reply); err != nil {
			return err
		}
		terminal(*grammar, "I have posted my reply in the topic.")
		return nil
	default:
		return fmt.Errorf("unknown mode %q", *mode)
	}
}

// narrate emits the pre-terminal events of the grammar — the "let me look at
// this" traffic a waker must never mistake for the answer.
func narrate(grammar string) {
	if grammar == "codex" {
		fmt.Println(`{"workdir":"/","provider":"mock","model":"harness-mock"}`)
		fmt.Println(`{"id":"1","msg":{"type":"task_started"}}`)
		fmt.Println(`{"id":"1","msg":{"type":"agent_message","message":"Let me look at the topic before answering."}}`)
		return
	}
	fmt.Println(`{"type":"system","subtype":"init"}`)
	fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"Let me look at the topic before answering."}]}}`)
}

// terminal emits the grammar's typed terminal event carrying the final text.
func terminal(grammar, text string) {
	switch grammar {
	case "codex":
		fmt.Printf(`{"id":"1","msg":{"type":"task_complete","last_agent_message":%q}}`+"\n", text)
	default:
		fmt.Printf(`{"type":"result","subtype":"success","result":%q}`+"\n", text)
	}
}

// selfPost posts the reply as the agent through the record itself — the mock
// acting the way a tool-wielding model does when it answers via its own door.
func selfPost(url, realmName, persona, topicPath, body string) error {
	if url == "" || realmName == "" || persona == "" || topicPath == "" {
		return fmt.Errorf("self-post mode needs --url, --realm, --persona and --topic")
	}
	nc, err := nats.Connect(url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer nc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := realm.NewClient(ctx, nc, realm.Config{Realm: realmName, Persona: persona})
	if err != nil {
		return fmt.Errorf("client: %w", err)
	}
	defer func() { _ = c.Close() }()
	if _, err := topic.Open(c, topicPath).PostTurn(ctx, body); err != nil {
		return fmt.Errorf("post: %w", err)
	}
	return nil
}

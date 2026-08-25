// Package wrap is how an agent answers mentions: one process, one agent,
// one credential, run on the machine where the person's assistant already
// lives — their logins, their configuration. The wrapper turns a mention of
// its persona into one harness invocation and guarantees the topic exactly
// one outcome per wake. It exists because attaching an agent should be one
// command on hardware the person already trusts; it is a subscriber and a
// client of the record, never a second control plane, and it keeps no
// state — not even a consumer: every outcome publishes under a
// deterministic id, so the record itself is the position.
//
// Every wake passes a budget before the harness runs (hq design 0006 —
// loop safety): a window floor on the persona's own recent turns in the
// topic, and a depth bound on the provable wake chain behind the trigger.
// Both compute from the topic view alone. A wake over budget is refused
// op-lessly and loudly — nothing posts, one wake_refused log line says
// why with the numbers, and the mention stays answerable: exhaustion is a
// delay, never a loss. Running without the budget is Config.Unbudgeted,
// an explicit standing logged once at startup.
package wrap

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/record"
	"github.com/impire-io/soulstream-core/topic"
)

// Wrapper serves one agent from one connection. Client must be bound to the
// agent's own persona — the connection's admission is the credential check,
// and everything posted is authored by that persona mechanically.
type Wrapper struct {
	Config Config
	Client *realm.Client
	Invoke Invoker      // nil = RunHarness
	Log    *slog.Logger // nil = slog.Default()
}

// Run catches up (the inbox's bounded window against the record's own
// outcome ids), then answers live mentions until ctx ends. Reconnects
// re-run catch-up, so a network blip cannot lose a wake still inside the
// window. Wakes run sequentially — a laptop is not a fleet.
func (w *Wrapper) Run(ctx context.Context) error {
	log := w.Log
	if log == nil {
		log = slog.Default()
	}
	invoke := w.Invoke
	if invoke == nil {
		invoke = RunHarness
	}
	if err := w.Config.Template.Validate(); err != nil {
		return err
	}
	if err := w.Config.Budget.Validate(); err != nil {
		return err
	}
	w.Config.ApplyDefaults()
	if w.Config.Unbudgeted || (w.Config.Budget.MaxHops == 0 && w.Config.Budget.WindowMax == 0) {
		// The explicit unbudgeted standing, stated once: no wake budget
		// means a cascade is bounded by nothing but the operator's eye.
		log.Info("wrap_unbudgeted", "persona", w.Config.Persona)
	}

	rlm := &agentRealm{client: w.Client}
	wakes := make(chan Wake, 64)

	// Live first, catch-up second: anything arriving between the two lands
	// in the channel, and the outcome-existence check makes overlap a no-op.
	sub, err := w.Client.Conn().Subscribe(topic.NotifySubject(w.Config.Persona), func(m *nats.Msg) {
		var n topic.NotifyPayload
		if err := parseNotify(m, &n); err != nil {
			log.Info("notify_skipped", "err", err)
			return
		}
		select {
		case wakes <- Wake{Topic: n.Topic, OpID: n.OpID, Author: n.Author}:
		default:
			log.Warn("notify_dropped_live", "topic", n.Topic, "op", n.OpID,
				"note", "the next catch-up answers it — the record is the position")
		}
	})
	if err != nil {
		return fmt.Errorf("wrap: subscribe notify: %w", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	catchUp := func(why string) {
		notes, err := topic.FetchInbox(ctx, w.Client, w.Config.Persona, w.Config.InboxLimit, nil)
		if err != nil {
			log.Error("catch_up_failed", "why", why, "err", err)
			return
		}
		log.Info("catch_up", "why", why, "inbox", len(notes))
		for i := len(notes) - 1; i >= 0; i-- { // oldest first
			n := notes[i]
			select {
			case wakes <- Wake{Topic: n.Topic, OpID: n.OpID, Author: n.Author}:
			case <-ctx.Done():
				return
			}
		}
	}
	reconnects := make(chan struct{}, 1)
	w.Client.Conn().SetReconnectHandler(func(*nats.Conn) {
		select {
		case reconnects <- struct{}{}:
		default:
		}
	})

	catchUp("start")
	log.Info("wrap_up", "persona", w.Config.Persona)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-reconnects:
			catchUp("reconnect")
		case wk := <-wakes:
			if _, err := handleWake(ctx, w.Config, rlm, invoke, wk, log); err != nil {
				// The realm was unreachable mid-wake; nothing was posted
				// without its id, so parking it for the next catch-up (or
				// reconnect) never duplicates.
				log.Error("wake_parked", "topic", wk.Topic, "op", wk.OpID, "err", err)
				time.Sleep(time.Second)
			}
		}
	}
}

// parseNotify decodes a live notify message, refusing non-mention types.
func parseNotify(m *nats.Msg, n *topic.NotifyPayload) error {
	if t := m.Header.Get(record.HeaderType); t != topic.TypeMentionNotify {
		return fmt.Errorf("not a mention: %q", t)
	}
	return json.Unmarshal(m.Data, n)
}

// agentRealm is Realm over the agent's own client: reads materialise, posts
// ride the idempotent turn arm, authorship is the client's persona.
type agentRealm struct {
	client *realm.Client
}

func (r *agentRealm) Read(ctx context.Context, topicPath string) ([]Turn, error) {
	view, err := topic.Open(r.client, topicPath).Materialise(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Turn, 0, len(view.Contributions))
	for _, c := range view.Contributions {
		out = append(out, Turn{OpID: c.OpID, Author: c.Author, Type: c.Type, Body: c.Body,
			Timestamp: c.Timestamp})
	}
	return out, nil
}

func (r *agentRealm) Post(ctx context.Context, topicPath, body string, mentions []string, opID string) (string, error) {
	return topic.Open(r.client, topicPath).PostTurnIdempotent(ctx, body, mentions, opID)
}

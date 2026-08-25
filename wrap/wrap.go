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
// The same engine runs a DECLARED agent (hq design 0005): a declaration's
// wake entries generalize the mention protocol to four kinds — mention and
// topic (replay-exact record wakes, answering where triggered), schedule
// (replay-exact off the system stream's server ticks, TTL-bounded backlog)
// and subject (at-most-once core NATS, honestly lossy) — all through the one
// identity rule (WakeOpID of the kind's trigger identity and the persona)
// and the one admission seam. Non-record wakes land outcomes on the declared
// home topic; a topic wake never fires on the persona's own ops; declared
// instructions are materialised from the record at every wake, never cached.
// Every read runs on the engine's own connection (the runtime-side-reads
// decision, specs/009). A future fleet dispatcher consumes this same
// package and inherits the admission seam (design 0006 §6).
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

// Run catches up (the record's own outcome ids against each source's
// backlog), then answers live wakes until ctx ends. Reconnects re-run
// catch-up, so a network blip cannot lose a stream-backed wake. Wakes run
// sequentially — a laptop is not a fleet. Without a declared wake set the
// engine is the mention-only wrapper, exactly as it always was; with one,
// exactly the declared sources run (mentions only if declared).
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
	if err := w.Config.Wakes.Validate(w.Config.HomeTopic); err != nil {
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
	tryEnqueue := func(wk Wake) bool {
		select {
		case wakes <- wk:
			return true
		default:
			return false
		}
	}
	blockEnqueue := func(ctx context.Context, wk Wake) bool {
		select {
		case wakes <- wk:
			return true
		case <-ctx.Done():
			return false
		}
	}

	set := w.Config.Wakes
	mentionOn := set == nil || set.Mention

	// Live first, catch-up second: anything arriving between the two lands
	// in the channel, and the outcome-existence check makes overlap a no-op.
	if mentionOn {
		sub, err := w.Client.Conn().Subscribe(topic.NotifySubject(w.Config.Persona), func(m *nats.Msg) {
			var n topic.NotifyPayload
			if err := parseNotify(m, &n); err != nil {
				log.Info("notify_skipped", "err", err)
				return
			}
			if !tryEnqueue(Wake{Kind: KindMention, Topic: n.Topic, OpID: n.OpID, Author: n.Author}) {
				log.Warn("notify_dropped_live", "topic", n.Topic, "op", n.OpID,
					"note", "the next catch-up answers it — the record is the position")
			}
		})
		if err != nil {
			return fmt.Errorf("wrap: subscribe notify: %w", err)
		}
		defer func() { _ = sub.Unsubscribe() }()
	}
	if set != nil {
		for _, tw := range set.Topics {
			sub, err := subscribeTopicWake(w.Client, w.Config.Persona, tw, tryEnqueue, log)
			if err != nil {
				return fmt.Errorf("wrap: subscribe topic wake %s: %w", tw.Path, err)
			}
			defer func() { _ = sub.Unsubscribe() }()
		}
		for _, sw := range set.Subjects {
			sub, err := subscribeSubjectWake(w.Client, w.Config.HomeTopic, sw, tryEnqueue, log)
			if err != nil {
				return fmt.Errorf("wrap: subscribe subject wake %s: %w", sw.Subject, err)
			}
			defer func() { _ = sub.Unsubscribe() }()
		}
		if len(set.Schedules) > 0 {
			// Reconcile the declaration onto the substrate, then consume the
			// ticks (backlog within TTL + live) — the server keeps ticking
			// whether or not the engine runs; outcome existence dedupes.
			if err := reconcileSchedules(ctx, w.Client, w.Config.Persona, set.Schedules); err != nil {
				return err
			}
			if err := consumeTicks(ctx, w.Client, w.Config.Persona, w.Config.HomeTopic,
				set.Schedules, blockEnqueue, log); err != nil {
				return err
			}
		}
	}

	catchUp := func(why string) {
		if mentionOn {
			notes, err := topic.FetchInbox(ctx, w.Client, w.Config.Persona, w.Config.InboxLimit, nil)
			if err != nil {
				log.Error("catch_up_failed", "why", why, "err", err)
			} else {
				log.Info("catch_up", "why", why, "inbox", len(notes))
				for i := len(notes) - 1; i >= 0; i-- { // oldest first
					n := notes[i]
					if !blockEnqueue(ctx, Wake{Kind: KindMention, Topic: n.Topic, OpID: n.OpID, Author: n.Author}) {
						return
					}
				}
			}
		}
		if set != nil {
			for _, tw := range set.Topics {
				catchUpTopicWake(ctx, w.Client, w.Config.Persona, tw, blockEnqueue, log)
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

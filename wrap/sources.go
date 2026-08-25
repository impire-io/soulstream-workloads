package wrap

// This file is the declared wake sources beyond mentions (hq design 0005 §3):
// topic wakes off the ops stream, schedule wakes off the system stream's
// server-generated ticks, subject wakes off plain core NATS. Every source
// only ENQUEUES — admission (self-skip, outcome existence, the 0006 budget)
// is handleWake's, identical for every kind. All reads run on the engine's
// own connection (the runtime-side-reads decision, specs/009): no durable
// consumers, no dispatcher state — outcome existence against the topic is
// the position for every stream-backed kind.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/record"
	"github.com/impire-io/soulstream-core/topic"
)

// typeSet resolves a topic wake's declared types (empty = turn.post) into a
// lookup set.
func typeSet(types []string) map[string]bool {
	if len(types) == 0 {
		return map[string]bool{"turn.post": true}
	}
	set := make(map[string]bool, len(types))
	for _, t := range types {
		set[t] = true
	}
	return set
}

// subscribeTopicWake attaches the live half of one topic wake: a plain
// subscription on the path's ops subject. Ops authored by the wrapped persona
// never enqueue — the normative self-exclusion, enforced at the source and
// again at admission.
func subscribeTopicWake(client *realm.Client, persona string, tw TopicWake, tryEnqueue func(Wake) bool, log *slog.Logger) (*nats.Subscription, error) {
	want := typeSet(tw.Types)
	return client.Conn().Subscribe(topic.OpsSubject(tw.Path), func(m *nats.Msg) {
		rec, err := record.Parse(m.Header, m.Data)
		if err != nil {
			log.Info("topic_op_skipped", "path", tw.Path, "err", err)
			return
		}
		if rec.Author == persona || !want[rec.Type] {
			return
		}
		if !tryEnqueue(Wake{Kind: KindTopic, Topic: tw.Path, OpID: rec.ID, Author: rec.Author, Body: attachmentBody(rec)}) {
			log.Warn("wake_dropped_live", "kind", KindTopic, "topic", tw.Path, "op", rec.ID,
				"note", "the next catch-up answers it — the record is the position")
		}
	})
}

// attachmentBody derives a prompt body for a live op that will not appear in
// the contributions view (an attachment.add); every other type anchors its
// body from the view in handleWake.
func attachmentBody(rec record.Record) string {
	if rec.Type != "attachment.add" {
		return ""
	}
	var p struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(rec.Payload, &p) == nil && p.Name != "" {
		return fmt.Sprintf("attachment %q added", p.Name)
	}
	return ""
}

// catchUpTopicWake replays one topic wake's backlog: materialise the path,
// enqueue every matching op whose outcome does not exist yet. The whole
// history is the backlog — replay-exact means outcome existence, not a
// window, is the position.
func catchUpTopicWake(ctx context.Context, client *realm.Client, persona string, tw TopicWake, enqueue func(context.Context, Wake) bool, log *slog.Logger) {
	view, err := topic.Open(client, tw.Path).Materialise(ctx)
	if err != nil {
		log.Error("catch_up_failed", "kind", KindTopic, "path", tw.Path, "err", err)
		return
	}
	want := typeSet(tw.Types)
	have := make(map[string]bool, len(view.Contributions))
	for _, c := range view.Contributions {
		have[c.OpID] = true
	}
	pending := 0
	for _, c := range view.Contributions {
		if c.Author == persona || !want[c.Type] {
			continue
		}
		if have[WakeOpID(c.OpID, persona)] {
			continue // answered — the record is the position
		}
		pending++
		if !enqueue(ctx, Wake{Kind: KindTopic, Topic: tw.Path, OpID: c.OpID, Author: c.Author}) {
			return
		}
	}
	if want["attachment.add"] {
		for _, a := range view.Attachments {
			if a.Author == persona || have[WakeOpID(a.OpID, persona)] {
				continue
			}
			pending++
			if !enqueue(ctx, Wake{Kind: KindTopic, Topic: tw.Path, OpID: a.OpID, Author: a.Author,
				Body: fmt.Sprintf("attachment %q added", a.Name)}) {
				return
			}
		}
	}
	log.Info("catch_up", "kind", KindTopic, "path", tw.Path, "pending", pending)
}

// reconcileSchedules converges the declaration's schedule entries onto the
// substrate: one headered registration message per entry on
// SOULSTREAM.SYSTEM.SCHEDULES.<persona>.<name> — the server appends ticks on
// the TICKS subject whether or not anything consumes. Re-publishing replaces;
// purging the registration subject deregisters. The registration's payload
// rides on every tick and becomes the wake's body.
func reconcileSchedules(ctx context.Context, client *realm.Client, persona string, entries []ScheduleWake) error {
	js := client.JetStream()
	for _, s := range entries {
		sched, err := realm.SystemScheduleSubject(persona, s.Name)
		if err != nil {
			return fmt.Errorf("wrap: %w", err)
		}
		target, err := realm.SystemTickSubject(persona, s.Name)
		if err != nil {
			return fmt.Errorf("wrap: %w", err)
		}
		msg := &nats.Msg{
			Subject: sched,
			Header: nats.Header{
				jetstream.ScheduleHeader:       []string{s.Pattern},
				jetstream.ScheduleTargetHeader: []string{target},
			},
			Data: []byte(fmt.Sprintf("schedule %q fired (%s)", s.Name, s.Pattern)),
		}
		if s.TTL > 0 {
			// The declared backlog bound: the scheduler stamps every tick
			// with this per-message TTL, so ticks the engine never reads
			// expire instead of piling up.
			msg.Header.Set(jetstream.ScheduleTTLHeader, s.TTL.String())
		}
		if _, err := js.PublishMsg(ctx, msg); err != nil {
			return fmt.Errorf("wrap: register schedule %q: %w", s.Name, err)
		}
	}
	return nil
}

// consumeTicks reads the schedules' tick subjects with an ordered (ephemeral)
// consumer from the start of the retained backlog — never durable: expired
// ticks are gone (the TTL bound), surviving ticks replay after a restart, and
// outcome existence dedupes what was already answered. The trigger identity
// is the tick's stream sequence, unique across the system stream.
func consumeTicks(ctx context.Context, client *realm.Client, persona, homeTopic string, entries []ScheduleWake, enqueue func(context.Context, Wake) bool, log *slog.Logger) error {
	subjects := make([]string, 0, len(entries))
	for _, s := range entries {
		t, err := realm.SystemTickSubject(persona, s.Name)
		if err != nil {
			return fmt.Errorf("wrap: %w", err)
		}
		subjects = append(subjects, t)
	}
	stream, err := client.JetStream().Stream(ctx, realm.SystemStreamName)
	if err != nil {
		return fmt.Errorf("wrap: this realm has no system stream yet — run `soulstream provision` to converge it: %w", err)
	}
	cons, err := stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		FilterSubjects: subjects,
		DeliverPolicy:  jetstream.DeliverAllPolicy,
	})
	if err != nil {
		return fmt.Errorf("wrap: tick consumer: %w", err)
	}
	it, err := cons.Messages()
	if err != nil {
		return fmt.Errorf("wrap: consume ticks: %w", err)
	}
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(it.Stop) }
	go func() {
		<-ctx.Done()
		stop()
	}()
	go func() {
		defer stop()
		for {
			msg, err := it.Next()
			if err != nil {
				if ctx.Err() == nil {
					log.Error("tick_consume_stopped", "err", err,
						"note", "surviving ticks replay on the next start — the record is the position")
				}
				return
			}
			md, err := msg.Metadata()
			if err != nil {
				continue
			}
			enqueue(ctx, Wake{Kind: KindSchedule, Topic: homeTopic,
				OpID: strconv.FormatUint(md.Sequence.Stream, 10), Body: string(msg.Data())})
		}
	}()
	return nil
}

// subscribeSubjectWake attaches one subject wake: a plain core-NATS
// subscription, at-most-once by design — no stream, no catch-up, and a wake
// arriving while the engine is down (or while the queue is full) is lost, as
// the declared delivery class states. The trigger identity is the payload's
// lowercase-hex SHA-256, so identical payloads collapse to one outcome.
func subscribeSubjectWake(client *realm.Client, homeTopic string, sw SubjectWake, tryEnqueue func(Wake) bool, log *slog.Logger) (*nats.Subscription, error) {
	return client.Conn().Subscribe(sw.Subject, func(m *nats.Msg) {
		sum := sha256.Sum256(m.Data)
		wk := Wake{Kind: KindSubject, Topic: homeTopic, OpID: hex.EncodeToString(sum[:]), Body: string(m.Data)}
		if !tryEnqueue(wk) {
			log.Warn("wake_dropped_live", "kind", KindSubject, "subject", sw.Subject,
				"note", "at-most-once — this wake is lost, as declared")
		}
	})
}

package waker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/impire-io/soulstream-core/record"
	"github.com/impire-io/soulstream-core/topic"
)

// The narrow seams the wake protocol runs over. Real occupants are core
// clients wired in waker.go and cmd/; tests use fakes and assert call
// sequences (the runner_test.go pattern).

// RealmOps is the waker's own view of the record: read a topic, and speak in
// the waker's voice. Both posts ride the idempotent arm — a wake has one
// outcome slot.
type RealmOps interface {
	Read(ctx context.Context, topicPath string) ([]Turn, error)
	PostAsWaker(ctx context.Context, topicPath, body string, mentions []string, opID string) (string, error)
}

// AgentSession is one admitted wake's standing as the agent: the dial that
// created it was the admission probe, and Post is authored by the agent
// because the session's client is — authorship is mechanical, never a
// parameter.
type AgentSession interface {
	Post(ctx context.Context, topicPath, body, opID string) (string, error)
	Close()
}

// AgentDialer dials one wake's agent session. Failure classes: ErrRefused
// (the registration was revoked — the wake is refused) versus anything else
// (the realm is unreachable — the wake retries sooner). The ephemeral lane's
// dialer mints before dialing.
type AgentDialer func(ctx context.Context, runDir string) (AgentSession, map[string]string, error)

// ErrRefused marks an admission refusal (authorization violation): the agent
// cannot speak, so the wake produces no op of any kind and waits for a
// re-grant.
var ErrRefused = fmt.Errorf("waker: admission refused")

// Invoker runs one harness invocation. The default is RunHarness; tests
// substitute.
type Invoker func(ctx context.Context, spec RunSpec) HarnessResult

// delivery is the slice of jetstream.Msg the protocol needs.
type delivery interface {
	Data() []byte
	Headers() nats.Header
	Metadata() (*jetstream.MsgMetadata, error)
	Ack() error
	NakWithDelay(time.Duration) error
}

// Nak delays per failure class: refused waits for an operator act, unreachable
// and retry are transient.
const (
	refusedDelay     = 15 * time.Second
	unreachableDelay = 3 * time.Second
	retryDelay       = 2 * time.Second
)

// handler serves one registration's wakes.
type handler struct {
	reg     Registration
	realm   RealmOps
	dial    AgentDialer
	invoke  Invoker
	scratch string
	log     *slog.Logger
}

// handle is the per-delivery protocol: pre-check, probe, materialise, invoke,
// discharge. The ack is the last act of an outcome, never earlier.
func (h *handler) handle(ctx context.Context, msg delivery) {
	if t := msg.Headers().Get(record.HeaderType); t != topic.TypeMentionNotify {
		// The notify subject is deliberately general; the waker consumes only
		// what it understands, visibly.
		h.log.Info("notify_skipped", "type", t)
		_ = msg.Ack()
		return
	}
	var n topic.NotifyPayload
	if err := json.Unmarshal(msg.Data(), &n); err != nil {
		h.log.Error("notify_unparseable", "err", err)
		_ = msg.Ack() // a malformed pointer never becomes parseable by retrying
		return
	}
	if n.Author == h.reg.Persona {
		// An agent's own mention of itself never wakes it — the measured
		// self-loop guard: without this, a reply or testimony tapping the
		// agent re-triggers the very machinery that produced it.
		h.log.Info("notify_self_skipped", "agent", h.reg.Persona, "topic", n.Topic)
		_ = msg.Ack()
		return
	}
	meta, err := msg.Metadata()
	if err != nil {
		h.log.Error("metadata", "err", err)
		_ = msg.NakWithDelay(unreachableDelay)
		return
	}
	delivered := int(meta.NumDelivered)
	wakeID := WakeOpID(n.OpID, h.reg.Persona)
	log := h.log.With("agent", h.reg.Persona, "topic", n.Topic, "op", n.OpID, "delivery", delivered)
	log.Info("wake")

	// Redelivery pre-check: an outcome that already landed is acked, not
	// re-run — the crash-after-post window, closed at any redelivery distance.
	if delivered > 1 {
		view, err := h.realm.Read(ctx, n.Topic)
		if err != nil {
			log.Error("read", "err", err)
			_ = msg.NakWithDelay(unreachableDelay)
			return
		}
		if ContainsOp(view, wakeID) {
			log.Info("outcome", "kind", "already_landed", "op_id", wakeID)
			_ = msg.Ack()
			return
		}
	}

	runDir := filepath.Join(h.scratch, fmt.Sprintf("%s-d%d", wakeID, delivered))

	// The dial is the admission probe: a refused agent cannot speak, so the
	// wake is refused — no harness, no op, redelivery when something changes.
	sess, mcpOverlay, err := h.dial(ctx, runDir)
	if err != nil {
		if isRefused(err) {
			log.Info("wake_refused", "err", err)
			_ = msg.NakWithDelay(refusedDelay)
		} else {
			log.Warn("wake_unreachable", "err", err)
			_ = msg.NakWithDelay(unreachableDelay)
		}
		return
	}
	defer sess.Close()

	before, err := h.realm.Read(ctx, n.Topic)
	if err != nil {
		log.Error("read", "err", err)
		_ = msg.NakWithDelay(unreachableDelay)
		return
	}
	prompt := fill(h.reg.Template.Prompt, map[string]string{
		"PERSONA": h.reg.Persona,
		"TOPIC":   n.Topic,
		"AUTHOR":  n.Author,
		"OP_ID":   n.OpID,
		"BODY":    AnchorBody(before, n.OpID),
	})

	res := h.invoke(ctx, RunSpec{
		Template:   h.reg.Template,
		Prompt:     prompt,
		Topic:      n.Topic,
		RunDir:     runDir,
		Timeout:    time.Duration(h.reg.RunTimeout),
		MCPOverlay: mcpOverlay,
	})
	log.Info("harness_done", "ok", res.OK, "detail", res.Detail)

	// Discharge on a context that survives shutdown: a waker that is going
	// down still finishes the wake it owes (the runner.Running pattern).
	post := context.WithoutCancel(ctx)

	after, err := h.realm.Read(post, n.Topic)
	if err != nil {
		log.Error("read", "err", err)
		_ = msg.NakWithDelay(unreachableDelay)
		return
	}
	if opID, already := PostedDuringRun(before, after, h.reg.Persona); already {
		log.Info("outcome", "kind", "correlated_self_post", "op_id", opID)
		_ = msg.Ack()
		return
	}
	if res.OK {
		opID, err := sess.Post(post, n.Topic, res.Text, wakeID)
		if err != nil {
			log.Error("post", "err", err)
			_ = msg.NakWithDelay(unreachableDelay)
			return
		}
		log.Info("outcome", "kind", "reply_posted", "op_id", opID)
		_ = msg.Ack()
		return
	}
	if delivered < h.reg.MaxDeliver {
		log.Info("retry", "of", h.reg.MaxDeliver)
		_ = msg.NakWithDelay(retryDelay)
		return
	}
	// The budget is spent. Failure is the waker's testimony about the agent,
	// spoken in the waker's own voice — the agent's voice is never the
	// failure channel (design 0004 §7). The turn NAMES the agent but taps
	// only the ASKER: tapping the agent would wake it to hear of its own
	// failure — the loop this package's integration gate measured.
	body := fmt.Sprintf("%s was asked and could not answer: %s (delivery %d/%d).",
		h.reg.Persona, res.Detail, delivered, h.reg.MaxDeliver)
	opID, err := h.realm.PostAsWaker(post, n.Topic, body, []string{n.Author}, wakeID)
	if err != nil {
		log.Error("post_failure_turn", "err", err)
		_ = msg.NakWithDelay(unreachableDelay)
		return
	}
	log.Info("outcome", "kind", "failure_posted", "op_id", opID)
	_ = msg.Ack()
}

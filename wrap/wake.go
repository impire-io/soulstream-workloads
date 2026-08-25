package wrap

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"
)

// Realm is the record as one wrapped agent sees it: read a topic, post as
// yourself. The real occupant is a core client bound to the agent's persona
// (authorship is mechanical — the client's persona is the author); tests use
// fakes and assert call sequences.
type Realm interface {
	Read(ctx context.Context, topicPath string) ([]Turn, error)
	Post(ctx context.Context, topicPath, body string, mentions []string, opID string) (string, error)
}

// Invoker runs one harness invocation. The default is RunHarness; tests
// substitute.
type Invoker func(ctx context.Context, spec RunSpec) HarnessResult

// InstructionSource materialises the agent's current instructions — the
// declared artefact lineage's tip, digest-checked, per wake, held in memory
// only (hq design 0005 §2: the runtime MUST NOT hold a durable copy). The
// shipped occupant is NewRecordInstructions; tests substitute.
type InstructionSource interface {
	Materialise(ctx context.Context) (string, error)
}

// WakeKind is which declared source produced a wake. The zero value is a
// legacy mention (the pre-declaration engine), byte-identical in behavior.
type WakeKind string

// The wake kinds and their delivery classes (hq design 0005 §2-3; the
// normative contract lives in specs/009 contracts/wake-kinds.md):
// mention and topic are replay-exact record wakes answering where triggered;
// schedule is replay-exact with a TTL-bounded tick backlog; subject is
// at-most-once. Non-record wakes land outcomes on the declared home topic.
const (
	KindMention  WakeKind = "mention"
	KindTopic    WakeKind = "topic"
	KindSchedule WakeKind = "schedule"
	KindSubject  WakeKind = "subject"
)

// Wake is one trigger as the engine works it. Topic is where the outcome
// lands (the triggering topic for record wakes, the declared home topic for
// schedule/subject). OpID is the kind's trigger identity — the string hashed
// with the persona into the one outcome id: the notify op id (mention), the
// triggering op id (topic), the tick's stream sequence as a decimal string
// (schedule), or the lowercase-hex SHA-256 of the payload (subject). Author
// is empty for non-record wakes — they have no asker. Body carries the
// trigger's text when the trigger is not an op in the topic (tick payload,
// subject payload); record wakes anchor their body from the view.
type Wake struct {
	Topic  string
	OpID   string
	Author string
	Kind   WakeKind
	Body   string
}

// retryDelay spaces harness attempts within one wake — in-process, because
// the wrapper has no redelivery machinery to lean on and needs none.
const retryDelay = 2 * time.Second

// handleWake answers one wake, or explains why not, and returns what it
// did (for the log and the tests): "answered", "self_reported",
// "already_answered", "self_skipped", "refused" (the wake budget spoke —
// op-less, loud, re-evaluated on any later delivery), or an error when the
// realm was unreachable (the caller may retry the whole wake later —
// nothing was posted without its deterministic id, so a retry never
// duplicates). Every kind passes the same seam order: self-skip →
// outcome-existence pre-check → budget → invoke → discharge.
func handleWake(ctx context.Context, cfg Config, realm Realm, invoke Invoker, w Wake, log *slog.Logger) (string, error) {
	kind := w.Kind
	if kind == "" {
		kind = KindMention
	}
	log = log.With("topic", w.Topic, "op", w.OpID, "author", w.Author, "wake", kind)
	if w.Author != "" && w.Author == cfg.Persona {
		// The measured self-loop guard, normative for topic wakes: a trigger
		// authored by the wrapped persona — its own mention, its own op —
		// never wakes it.
		log.Info("wake_self_skipped")
		return "self_skipped", nil
	}
	wakeID := WakeOpID(w.OpID, cfg.Persona)

	before, err := realm.Read(ctx, w.Topic)
	if err != nil {
		return "", fmt.Errorf("wrap: read %s: %w", w.Topic, err)
	}
	if ContainsOp(before, wakeID) {
		log.Info("wake_already_answered", "outcome", wakeID)
		return "already_answered", nil
	}

	if !cfg.Unbudgeted {
		// The wake budget (hq design 0006) sits here — after the self-skip
		// and the outcome-existence pre-check, before the outcome obligation
		// attaches — for every kind. A refusal is op-less (a refusal that
		// posts is a wake source — the measured failure ping-pong) and loud;
		// nothing is acked away, so a later catch-up re-evaluates against a
		// slid window: exhaustion is a delay, never a loss. A non-record
		// trigger is absent from the view and walks as a chain root; the
		// window floor applies to it in full.
		trigger := Turn{OpID: w.OpID, Author: w.Author}
		for _, t := range before {
			if t.OpID == w.OpID {
				trigger = t
				break
			}
		}
		if reason, refuse := BudgetDecision(cfg.Budget, before, trigger, cfg.Persona, time.Now()); refuse {
			log.Warn("wake_refused", "reason", reason)
			return "refused", nil
		}
	}

	// The declared instructions are materialised fresh for every wake — the
	// lineage tip out of the record, digest-checked, in memory only. A
	// failure parks the wake loudly (error return → wake_parked): the
	// trigger stays answerable, and an agent must not run on stale or
	// unverifiable instructions.
	instructions := ""
	if cfg.Instructions != nil {
		instructions, err = cfg.Instructions.Materialise(ctx)
		if err != nil {
			return "", fmt.Errorf("wrap: materialise instructions: %w", err)
		}
	}

	body := AnchorBody(before, w.OpID)
	if body == "" {
		body = w.Body
	}
	prompt := fill(cfg.Template.Prompt, map[string]string{
		"PERSONA":      cfg.Persona,
		"TOPIC":        w.Topic,
		"AUTHOR":       w.Author,
		"OP_ID":        w.OpID,
		"BODY":         body,
		"KIND":         string(kind),
		"INSTRUCTIONS": instructions,
	})

	// Discharge rides a context that survives shutdown: a wrapper going down
	// still finishes the wake it started.
	post := context.WithoutCancel(ctx)

	// The failure self-report taps the asker — the mentioning author, or the
	// triggering op's author on a topic wake. Schedule and subject wakes have
	// no asker: their self-report stands alone on the home topic.
	var taps []string
	if w.Author != "" {
		taps = []string{w.Author}
	}

	var lastDetail string
	for attempt := 1; attempt <= cfg.Retries; attempt++ {
		runDir := filepath.Join(cfg.Scratch, fmt.Sprintf("%s-a%d", wakeID, attempt))
		res := invoke(ctx, RunSpec{
			Template: cfg.Template,
			Prompt:   prompt,
			Topic:    w.Topic,
			RunDir:   runDir,
			Timeout:  cfg.RunTimeout,
		})
		log.Info("harness_done", "attempt", attempt, "ok", res.OK, "detail", res.Detail)

		after, err := realm.Read(post, w.Topic)
		if err != nil {
			return "", fmt.Errorf("wrap: read %s: %w", w.Topic, err)
		}
		if opID, already := PostedDuringRun(before, after, cfg.Persona); already {
			log.Info("outcome", "kind", "correlated_self_post", "op_id", opID)
			return "answered", nil
		}
		if res.OK {
			opID, err := realm.Post(post, w.Topic, res.Text, nil, wakeID)
			if err != nil {
				return "", fmt.Errorf("wrap: post reply: %w", err)
			}
			log.Info("outcome", "kind", "reply_posted", "op_id", opID)
			return "answered", nil
		}
		lastDetail = res.Detail
		before = after
		if attempt < cfg.Retries {
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}

	// The budget is spent: the agent reports its own failure — same voice
	// that would have replied, tapping only the asker (tapping the wrapped
	// persona was the measured runaway; tapping yourself is absurd), or
	// nobody when the wake had none.
	body = fmt.Sprintf("I was asked and could not answer: %s (%d attempts).",
		lastDetail, cfg.Retries)
	opID, err := realm.Post(post, w.Topic, body, taps, wakeID)
	if err != nil {
		return "", fmt.Errorf("wrap: post self-report: %w", err)
	}
	log.Info("outcome", "kind", "self_reported", "op_id", opID)
	return "self_reported", nil
}

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

// Wake is one mention as the wrapper works it.
type Wake struct {
	Topic  string
	OpID   string
	Author string
}

// retryDelay spaces harness attempts within one wake — in-process, because
// the wrapper has no redelivery machinery to lean on and needs none.
const retryDelay = 2 * time.Second

// handleWake answers one mention, or explains why not, and returns what it
// did (for the log and the tests): "answered", "self_reported",
// "already_answered", "self_skipped", or an error when the realm was
// unreachable (the caller may retry the whole wake later — nothing was
// posted without its deterministic id, so a retry never duplicates).
func handleWake(ctx context.Context, cfg Config, realm Realm, invoke Invoker, w Wake, log *slog.Logger) (string, error) {
	log = log.With("topic", w.Topic, "op", w.OpID, "author", w.Author)
	if w.Author == cfg.Persona {
		// The measured self-loop guard: an agent's own mention of itself —
		// in a reply it wrote, in its own self-report — never wakes it.
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

	prompt := fill(cfg.Template.Prompt, map[string]string{
		"PERSONA": cfg.Persona,
		"TOPIC":   w.Topic,
		"AUTHOR":  w.Author,
		"OP_ID":   w.OpID,
		"BODY":    AnchorBody(before, w.OpID),
	})

	// Discharge rides a context that survives shutdown: a wrapper going down
	// still finishes the wake it started.
	post := context.WithoutCancel(ctx)

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
	// persona was the measured runaway; tapping yourself is absurd).
	body := fmt.Sprintf("I was asked and could not answer: %s (%d attempts).",
		lastDetail, cfg.Retries)
	opID, err := realm.Post(post, w.Topic, body, []string{w.Author}, wakeID)
	if err != nil {
		return "", fmt.Errorf("wrap: post self-report: %w", err)
	}
	log.Info("outcome", "kind", "self_reported", "op_id", opID)
	return "self_reported", nil
}

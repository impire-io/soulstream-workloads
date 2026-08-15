package waker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	natsjwt "github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nkeys"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"
)

// EphemeralMinter mints a per-run credential against a declared role — the
// identity plane's mint.ephemeral op. The waker mints for the agent; an agent
// cannot mint for itself (the op is operator-gated). The real occupant is the
// soulstream-identity client, wired in cmd/; tests fake it.
type EphemeralMinter interface {
	MintEphemeral(role, user, userPublicKey string, ttl time.Duration, tags []string) (string, error)
}

// Waker serves a realm's registered agents: one durable consumer per agent on
// the notify stream, the wake protocol per delivery.
type Waker struct {
	Config Config
	Client *realm.Client   // the waker's own standing: consumer creation + its persona's voice
	Minter EphemeralMinter // nil unless a registration declares the ephemeral lane
	Invoke Invoker         // nil = RunHarness
	Log    *slog.Logger    // nil = slog.Default()
}

// Serve consumes wakes until ctx ends. Mentions posted while it was down are
// the backlog it drains first, in stream order, per persona.
func (w *Waker) Serve(ctx context.Context) error {
	log := w.Log
	if log == nil {
		log = slog.Default()
	}
	invoke := w.Invoke
	if invoke == nil {
		invoke = RunHarness
	}
	for _, r := range w.Config.Agents {
		if r.EphemeralLane() && w.Minter == nil {
			return fmt.Errorf("waker: agent %q declares the ephemeral lane but no minter is wired", r.Persona)
		}
	}

	js := w.Client.JetStream()
	var wg sync.WaitGroup
	for _, reg := range w.Config.Agents {
		cons, err := js.CreateOrUpdateConsumer(ctx, realm.NotifyStreamName, jetstream.ConsumerConfig{
			Durable:       "waker-" + reg.Persona,
			FilterSubject: topic.NotifySubject(reg.Persona),
			AckPolicy:     jetstream.AckExplicitPolicy,
			// AckWait must outlive the run plus the discharge; redelivery
			// beyond it is the crash window the pre-check closes.
			AckWait: time.Duration(reg.RunTimeout) + 2*time.Minute,
			// The retry budget is waker policy (max_deliver); a server-side
			// cap would drop a wake silently, which constitution V forbids.
			MaxDeliver: -1,
		})
		if err != nil {
			return fmt.Errorf("waker: consumer for %q: %w", reg.Persona, err)
		}
		h := &handler{
			reg:     reg,
			realm:   &realmOps{client: w.Client},
			dial:    w.dialer(reg),
			invoke:  invoke,
			scratch: w.Config.Waker.Scratch,
			log:     log,
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			serveConsumer(ctx, cons, h)
		}()
	}
	log.Info("waker_up", "realm", w.Config.Waker.Realm, "agents", len(w.Config.Agents))
	wg.Wait()
	return ctx.Err()
}

// serveConsumer is one registration's fetch loop.
func serveConsumer(ctx context.Context, cons jetstream.Consumer, h *handler) {
	for {
		if ctx.Err() != nil {
			return
		}
		batch, err := cons.Fetch(1, jetstream.FetchMaxWait(3*time.Second))
		if err != nil {
			continue
		}
		for msg := range batch.Messages() {
			h.handle(ctx, msg)
		}
	}
}

// realmOps is RealmOps over the waker's own client.
type realmOps struct {
	client *realm.Client
}

func (r *realmOps) Read(ctx context.Context, topicPath string) ([]Turn, error) {
	view, err := topic.Open(r.client, topicPath).Materialise(ctx)
	if err != nil {
		return nil, fmt.Errorf("waker: materialise %s: %w", topicPath, err)
	}
	return turnsOf(view), nil
}

func (r *realmOps) PostAsWaker(ctx context.Context, topicPath, body string, mentions []string, opID string) (string, error) {
	return topic.Open(r.client, topicPath).PostTurnIdempotent(ctx, body, mentions, opID)
}

func turnsOf(view *topic.MaterializedTopic) []Turn {
	out := make([]Turn, 0, len(view.Contributions))
	for _, c := range view.Contributions {
		out = append(out, Turn{OpID: c.OpID, Author: c.Author, Type: c.Type, Body: c.Body})
	}
	return out
}

// dialer builds the registration's AgentDialer: the dial is the admission
// probe in every lane; the lanes only decide what the dial presents.
func (w *Waker) dialer(reg Registration) AgentDialer {
	cred := reg.Credential
	realmName := w.Config.Waker.Realm
	switch {
	case cred.Ephemeral != nil:
		minter := w.Minter
		lane := cred.Ephemeral
		return func(ctx context.Context, runDir string) (AgentSession, map[string]string, error) {
			credsPath, err := mintRunCredential(minter, lane, reg.Persona, runDir)
			if err != nil {
				return nil, nil, err
			}
			sess, err := dialAgent(ctx, realm.Config{
				URL: cred.URL, CredsFile: credsPath, Realm: realmName, Persona: reg.Persona,
			})
			if err != nil {
				return nil, nil, err
			}
			// The harness rides the same run-bounded credential; the token
			// entry is cleared in case the template's static env carried one.
			return sess, map[string]string{"SOULSTREAM_CREDS": credsPath, "SOULSTREAM_TOKEN": ""}, nil
		}
	case cred.Token != "":
		return func(ctx context.Context, _ string) (AgentSession, map[string]string, error) {
			sess, err := dialAgent(ctx, realm.Config{
				URL: cred.URL, CredsFile: cred.SentinelCreds, Token: cred.Token,
				Realm: realmName, Persona: reg.Persona,
			})
			return sess, nil, err
		}
	default:
		return func(ctx context.Context, _ string) (AgentSession, map[string]string, error) {
			sess, err := dialAgent(ctx, realm.Config{
				URL: cred.URL, Realm: realmName, Persona: reg.Persona,
			})
			return sess, nil, err
		}
	}
}

// mintRunCredential mints the run-bounded credential and writes it, 0600,
// into the run directory — scratch, reaped with the run, never at rest in
// configuration.
func mintRunCredential(minter EphemeralMinter, lane *EphemeralLane, persona, runDir string) (string, error) {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", fmt.Errorf("waker: run dir: %w", err)
	}
	kp, err := nkeys.CreateUser()
	if err != nil {
		return "", fmt.Errorf("waker: run keypair: %w", err)
	}
	pub, err := kp.PublicKey()
	if err != nil {
		return "", fmt.Errorf("waker: run keypair: %w", err)
	}
	jwtStr, err := minter.MintEphemeral(lane.Role, persona, pub, time.Duration(lane.TTL), nil)
	if err != nil {
		return "", fmt.Errorf("waker: mint ephemeral for %q: %w", persona, err)
	}
	seed, err := kp.Seed()
	if err != nil {
		return "", fmt.Errorf("waker: run keypair: %w", err)
	}
	creds, err := natsjwt.FormatUserConfig(jwtStr, seed)
	if err != nil {
		return "", fmt.Errorf("waker: assemble run credential: %w", err)
	}
	path := filepath.Join(runDir, "run.creds")
	if err := os.WriteFile(path, creds, 0o600); err != nil {
		return "", fmt.Errorf("waker: write run credential: %w", err)
	}
	return path, nil
}

// agentSession is AgentSession over a per-wake realm client.
type agentSession struct {
	client *realm.Client
}

func dialAgent(ctx context.Context, cfg realm.Config) (AgentSession, error) {
	c, err := realm.Connect(ctx, cfg)
	if err != nil {
		if isAuthErr(err) {
			return nil, fmt.Errorf("%w: %v", ErrRefused, err)
		}
		return nil, err
	}
	return &agentSession{client: c}, nil
}

func (s *agentSession) Post(ctx context.Context, topicPath, body, opID string) (string, error) {
	return topic.Open(s.client, topicPath).PostTurnIdempotent(ctx, body, nil, opID)
}

func (s *agentSession) Close() { _ = s.client.Close() }

// isRefused classifies a dial error as an admission refusal.
func isRefused(err error) bool { return errors.Is(err, ErrRefused) }

// isAuthErr recognises the server's admission refusal in a connect error.
func isAuthErr(err error) bool {
	if errors.Is(err, nats.ErrAuthorization) {
		return true
	}
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "authorization violation")
}

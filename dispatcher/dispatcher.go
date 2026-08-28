// Package dispatcher is the standing serve arm (hq design
// 02-DESIGN/soulstream-workloads/0007-agents-as-infrastructure.md): one
// process per fleet node that makes submit-and-forget real. It composes
// two shipped mechanisms and invents nothing — no coordinator, no
// consumer state beside the log, no new realm vocabulary. Submission is
// still fleet.Submit: an ordinary work item carrying a declaration.
//
// The loop, every part a mechanism that already ships:
//
//   - WATCH the placement topic live — a subscription on its ops
//     subject — with a materialise poll as catch-up only.
//   - RACE open placements through the ordinary claim path: claim,
//     re-materialise, serve only if the read-back names this node owner.
//     First claim in stream order wins, the rest fold void.
//   - RESUME on start every placement the log says this node owns, with
//     no new op and no handshake: the record is the position.
//   - SERVE each owned agent placement by running the specs/009 wake
//     engine for the declared persona. The engine brings catch-up,
//     exactly-once outcomes under the deterministic wake id, and the
//     0006 budget at admission with it — the dispatcher adds no
//     admission point of its own.
//   - ANSWER probes for what it serves and SWEEP peers' silent claims,
//     so a dead node's placements reclaim as an ordinary work.abandon
//     and reopen for a fresh race (design 0003 §6, unchanged).
//
// The serve seam's open question (design 0007 §2) is resolved here as
// option (b): this package owns its claim path rather than fleet.Node
// growing a launch hook, because fleet.Node.TryPlace hardwires
// Runner.Launch — correct for backend workloads — while its probe and
// sweep halves need no Runner at all. fleet is otherwise untouched, so a
// dispatcher node and a runner node share one realm and one placement
// topic without either knowing about the other.
//
// Credentials are deliberately not this package's business (design 0007
// §5): the client an engine runs on arrives through the caller's
// ConnectAgent hook. Whether that is a minted ephemeral, a creds file,
// or a callout token is the product's founding ceremony.
//
// Stopping is a deliberate choice, never chance (design 0007 §6). Drain
// stops taking work, cancels the engines and WAITS, so an in-flight
// harness failure lands the agent's own self-report; a hard stop is the
// process dying, which posts nothing and leaves the successor — a
// restart, or a peer via reclaim — to serve that wake exactly once.
package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/fleet"
	"github.com/impire-io/soulstream-workloads/wrap"
)

// ConnectFunc yields a realm client bound to one agent's persona. It is
// the whole credential seam: the dispatcher never resolves, holds, or
// injects credential material, and the returned connection's admission
// is the served agent's entire authority (wrap's standing, unchanged).
// The dispatcher closes the client when the engine stops.
type ConnectFunc func(ctx context.Context, persona string) (*realm.Client, error)

// Defaults for the cadences an operator rarely sets. The reclaim bound
// and probe timeout mirror fleet's own so a dispatcher and a runner node
// sweep each other on the same clock.
const (
	defaultReclaim      = 10 * time.Second
	defaultPollEvery    = 30 * time.Second
	defaultRaceBackoff  = 30 * time.Second
	defaultDrainTimeout = 60 * time.Second
	minSweepEvery       = 100 * time.Millisecond
)

// Dispatcher is one node's standing serve loop. Every field but the
// cadences is required; a zero cadence takes the default beside it.
type Dispatcher struct {
	// Node names this node — the persona its claims are attributed to
	// and the address peers probe. Empty takes the client's persona;
	// naming a different one is refused, because authorship is
	// mechanical and a claim cannot be attributed to a name the
	// connection does not hold.
	Node string
	// Client is the node's own connection: it materialises the placement
	// topic, publishes claims and abandons, and carries probe traffic.
	Client *realm.Client
	// Placements is the topic path submissions land on.
	Placements string
	// ConnectAgent yields the client one served agent's engine runs on.
	ConnectAgent ConnectFunc
	// Engine carries everything a declaration does not say — the harness
	// template, the scratch root, timeouts and retries. Its Persona must
	// be empty: one dispatcher serves whatever personas its placements
	// declare.
	Engine wrap.Config
	// EngineFor, when set, yields the engine config for ONE persona and
	// Engine becomes the fallback for personas it declines (nil config,
	// nil error). It exists because the tool door's credential is
	// per-agent while Engine is per-node (episode 0143, finding 3): the
	// template's MCP lane carries the persona's own authority, and one
	// base config cannot hold many. An error refuses the placement —
	// handed back for another node, never half-served.
	EngineFor func(ctx context.Context, persona string) (*wrap.Config, error)
	// Invoke runs one harness invocation. Nil is wrap's real harness.
	Invoke wrap.Invoker

	// Reclaim bounds how long a peer's claimed placement may go
	// unanswered before this node nominates it. Zero: 10s.
	Reclaim time.Duration
	// ProbeTimeout bounds one liveness probe. Zero: 250ms. It must fit
	// inside the reclaim bound (design 0003 §6).
	ProbeTimeout time.Duration
	// SweepEvery is the reclaim pass cadence. Zero: a quarter of the
	// reclaim bound, so a dead peer is nominated within it.
	SweepEvery time.Duration
	// PollEvery is the catch-up cadence — the fallback behind the live
	// subscription, never the mechanism (design 0007 §9). Zero: 30s.
	PollEvery time.Duration
	// RaceBackoff is how long this node leaves a placement it could not
	// serve alone. Node-local and transient: it delays a decision, never
	// makes one, and without it an unservable declaration becomes a
	// claim/abandon spin on the record. Zero: 30s.
	RaceBackoff time.Duration
	// DrainTimeout bounds the wait in the stop ceremony Run performs.
	// Zero: 60s.
	DrainTimeout time.Duration

	Log *slog.Logger // nil = slog.Default()

	mu      sync.Mutex
	node    string // resolved at Run: Node, or the client's persona
	taking  bool   // false before Run and after Drain: no work is taken on
	served  map[string]*serve
	backoff map[string]time.Time
}

// serve is one running engine: the placement it answers for, the persona
// it is bound to, and the two handles the stop ceremony needs.
type serve struct {
	persona string
	cancel  context.CancelFunc
	done    chan struct{}
}

// Servable reports whether the wake engine can serve this declaration:
// an agent with something that wakes it. Everything else — a tool, an
// agent with no wake set — belongs to the fleet's Runner path, and the
// dispatcher leaves it alone rather than claiming work it cannot run
// (design 0003 §2's self-selection, design 0007 §2's rule that the
// declared role and wake set decide engine-serve vs backend-launch).
func Servable(d declaration.Declaration) bool {
	return d.Role == declaration.RoleAgent && len(d.Wake) > 0
}

// Run serves until ctx ends, then drains and returns. A drain that could
// not finish inside DrainTimeout returns that failure; an ordinary stop
// returns nil — ending on a signal is what a daemon does, not an error.
//
// One Dispatcher runs once at a time; a second concurrent Run would
// claim under the same node name and probe on the same subject.
func (d *Dispatcher) Run(ctx context.Context) error {
	if err := d.start(); err != nil {
		return err
	}
	log := d.log()

	probes, err := d.Client.Conn().Subscribe(fleet.ProbeSubject(d.node), d.answerProbe)
	if err != nil {
		return fmt.Errorf("dispatcher: probe subscription: %w", err)
	}
	defer func() { _ = probes.Unsubscribe() }()

	scan := make(chan struct{}, 1)
	poke := func() {
		select {
		case scan <- struct{}{}:
		default:
		}
	}
	// Live first: any op on the placement topic — a submission, a peer's
	// abandon, our own reclaim — asks for one scan. The poll behind it is
	// catch-up, so a missed message costs a cadence and never a placement.
	ops, err := d.Client.Conn().Subscribe(topic.OpsSubject(d.Placements), func(*nats.Msg) { poke() })
	if err != nil {
		return fmt.Errorf("dispatcher: watch %s: %w", d.Placements, err)
	}
	defer func() { _ = ops.Unsubscribe() }()
	if err := d.Client.Conn().Flush(); err != nil {
		return fmt.Errorf("dispatcher: flush subscriptions: %w", err)
	}

	h := topic.Open(d.Client, d.Placements)
	sweeper := &fleet.Node{ID: d.node, Conn: d.Client.Conn(),
		Reclaim: d.Reclaim, ProbeTimeout: d.ProbeTimeout}

	poll := time.NewTicker(d.pollEvery())
	defer poll.Stop()
	sweep := time.NewTicker(d.sweepEvery())
	defer sweep.Stop()

	log.Info("dispatcher_up", "node", d.node, "placements", d.Placements)
	poke() // the first scan is the resume pass: the log says what we own

	for {
		select {
		case <-ctx.Done():
			return d.drainOnStop(ctx)
		case <-scan:
			d.scan(ctx, h)
		case <-poll.C:
			d.scan(ctx, h)
		case <-sweep.C:
			if d.sweepOnce(ctx, sweeper, h) {
				poke()
			}
		}
	}
}

// Drain is the stop ceremony (hq design 0007 §6): the dispatcher stops
// taking work on, cancels every engine, and waits for each — so an
// in-flight harness returns a failure and the engine posts the agent's
// own self-report before the process ends. Idempotent, and safe before
// Run. The other end is not offered here: crash semantics are the
// process dying, and a supervisor that wants them kills it.
//
// Placements stay claimed on the record across a drain. That is the
// point: this node's restart resumes them, and if it never returns a
// peer's sweep reclaims them into a fresh race.
func (d *Dispatcher) Drain(ctx context.Context) error {
	d.mu.Lock()
	d.taking = false
	stopping := make([]*serve, 0, len(d.served))
	for id, s := range d.served {
		stopping = append(stopping, s)
		delete(d.served, id)
	}
	d.mu.Unlock()

	for _, s := range stopping {
		s.cancel()
	}
	for _, s := range stopping {
		select {
		case <-s.done:
		case <-ctx.Done():
			return fmt.Errorf("dispatcher: drain did not finish — %d engine(s) still running: %w",
				len(stopping), ctx.Err())
		}
	}
	if len(stopping) > 0 {
		d.log().Info("dispatcher_drained", "engines", len(stopping))
	}
	return nil
}

// drainOnStop runs the stop ceremony on a context the ended one cannot
// cut short — the drain's whole purpose is to outlive the signal that
// asked for it.
func (d *Dispatcher) drainOnStop(ctx context.Context) error {
	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), d.drainTimeout())
	defer cancel()
	return d.Drain(drainCtx)
}

// start resolves the node name and refuses a configuration that could
// only fail later, one placement at a time.
func (d *Dispatcher) start() error {
	if d.Client == nil {
		return errors.New("dispatcher: a node client is required — claims are attributed to its persona")
	}
	if d.Placements == "" {
		return errors.New("dispatcher: a placement topic path is required")
	}
	if d.ConnectAgent == nil {
		return errors.New("dispatcher: a ConnectAgent hook is required — this package resolves no credentials of its own")
	}
	if d.Engine.Persona != "" {
		return fmt.Errorf("dispatcher: the engine config must not fix a persona (got %q) — one dispatcher serves whatever personas its placements declare",
			d.Engine.Persona)
	}
	if d.Engine.Scratch == "" {
		return errors.New("dispatcher: a scratch root is required — every served placement runs under its own directory")
	}
	if err := d.Engine.Template.Validate(); err != nil {
		return err
	}
	if d.ProbeTimeout >= d.reclaimBound() {
		return fmt.Errorf("dispatcher: probe timeout %s does not fit inside the reclaim bound %s — a probe that outlasts the window it guards decides nothing (design 0003 §6)",
			d.ProbeTimeout, d.reclaimBound())
	}
	node := d.Node
	if node == "" {
		node = d.Client.Persona()
	}
	if node == "" {
		return errors.New("dispatcher: a node name is required — the client carries no persona to take it from")
	}
	if p := d.Client.Persona(); p != "" && p != node {
		return fmt.Errorf("dispatcher: node %q cannot claim on a connection admitted as %q — authorship is mechanical", node, p)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.node = node
	d.taking = true
	d.served = map[string]*serve{}
	d.backoff = map[string]time.Time{}
	return nil
}

// scan is one pass over the placement topic from a single materialise:
// resume what the log says we own, race what is open. Both halves read
// the same view, so a placement that changed hands mid-scan is corrected
// by the claim path's read-back or by the next scan.
func (d *Dispatcher) scan(ctx context.Context, h *topic.Handle) {
	if !d.acceptingWork() {
		return
	}
	mt, err := h.Materialise(ctx)
	if err != nil {
		d.log().Error("placement_scan_failed", "err", err)
		return
	}
	for _, item := range mt.WorkItems {
		decl, ok := fleet.DeclarationOf(item)
		if !ok || !Servable(decl) {
			continue
		}
		switch {
		case item.Status == topic.WorkClaimed && item.Owner == d.node:
			// Resume: no op, no handshake, no local state — the record is
			// the position, on a first scan and on every one after.
			d.startServe(ctx, h, item.ID, decl, "resume")
		case item.Status == topic.WorkOpen:
			d.race(ctx, h, item.ID, decl)
		}
	}
}

// race is the claim path this package owns (the resolved seam, design
// 0007 §2 option (b)): publish an ordinary claim, read the log back, and
// serve only if it names this node. A lost race is the ordinary outcome
// of contention and never worth a retry — the losing claim folds void
// and stays visible as history.
func (d *Dispatcher) race(ctx context.Context, h *topic.Handle, itemID string, decl declaration.Declaration) {
	if d.skipRace(itemID, decl.Persona) {
		return
	}
	if _, err := h.ClaimWork(ctx, itemID); err != nil {
		d.log().Error("placement_claim_failed", "placement", itemID, "err", err)
		return
	}
	mt, err := h.Materialise(ctx)
	if err != nil {
		d.log().Error("placement_readback_failed", "placement", itemID, "err", err)
		return
	}
	item, ok := findItem(mt, itemID)
	if !ok || item.Status != topic.WorkClaimed || item.Owner != d.node {
		return
	}
	d.startServe(ctx, h, itemID, decl, "won")
}

func findItem(mt *topic.MaterializedTopic, id string) (topic.WorkItem, bool) {
	for _, item := range mt.WorkItems {
		if item.ID == id {
			return item, true
		}
	}
	return topic.WorkItem{}, false
}

// startServe puts one owned placement under a running wake engine. Every
// failure before the engine is up hands the placement straight back, so
// a placement is never half-served and another node may try.
func (d *Dispatcher) startServe(ctx context.Context, h *topic.Handle, itemID string, decl declaration.Declaration, how string) {
	log := d.log().With("placement", itemID, "persona", decl.Persona)
	if free, why := d.slotFree(itemID, decl.Persona); !free {
		if why != "" {
			log.Warn("placement_not_served", "why", why)
		}
		return
	}

	client, err := d.ConnectAgent(ctx, decl.Persona)
	if err != nil {
		d.handBack(ctx, h, itemID, log, fmt.Errorf("connect as %s: %w", decl.Persona, err))
		return
	}
	base := d.Engine
	if d.EngineFor != nil {
		per, err := d.EngineFor(ctx, decl.Persona)
		if err != nil {
			_ = client.Close()
			d.handBack(ctx, h, itemID, log, fmt.Errorf("engine for %s: %w", decl.Persona, err))
			return
		}
		if per != nil {
			base = *per
		}
	}
	base.Scratch = filepath.Join(base.Scratch, itemID)
	cfg, err := wrap.DeclaredConfig(base, decl, client)
	if err != nil {
		_ = client.Close()
		d.handBack(ctx, h, itemID, log, err)
		return
	}

	engineCtx, cancel := context.WithCancel(context.Background())
	s := &serve{persona: decl.Persona, cancel: cancel, done: make(chan struct{})}
	if !d.take(itemID, s) {
		// A drain landed between the checks and here.
		cancel()
		_ = client.Close()
		return
	}
	w := &wrap.Wrapper{Config: cfg, Client: client, Invoke: d.Invoke, Log: log}
	go func() {
		defer close(s.done)
		defer func() { _ = client.Close() }()
		err := w.Run(engineCtx)
		if engineCtx.Err() != nil {
			return // cancelled by the stop ceremony: the ordinary end
		}
		// The engine stopped on its own. Leaving the served set stops our
		// probe answers for this placement, so a peer's sweep reclaims it
		// into a fresh race; the backoff keeps our own retry off a hot
		// loop. Either way the record decides who serves it next.
		d.release(itemID)
		log.Error("engine_stopped", "err", err,
			"note", "the placement returns to the race — this node retries after the backoff")
	}()
	log.Info("placement_served", "how", how, "wakes", len(decl.Wake))
}

// handBack abandons a placement this node won but cannot serve — the
// discipline fleet.TryPlace applies to a failed launch — and marks it
// backed off so this node does not immediately re-race what it just
// failed at.
func (d *Dispatcher) handBack(ctx context.Context, h *topic.Handle, itemID string, log *slog.Logger, cause error) {
	d.backOff(itemID)
	log.Error("placement_unservable", "err", cause,
		"note", "handed back for another node — a placement is never half-served")
	if _, err := h.AbandonWork(ctx, itemID); err != nil {
		log.Error("placement_abandon_failed", "err", err)
	}
}

// sweepOnce runs fleet's reclaim pass with a Runner-less node: projection
// nominates a silent owner, a probe vetoes a live one, and an ordinary
// abandon reopens the rest. Reports whether anything reopened, so the
// caller can scan without waiting for the poll.
func (d *Dispatcher) sweepOnce(ctx context.Context, sweeper *fleet.Node, h *topic.Handle) bool {
	reopened, err := sweeper.Sweep(ctx, h)
	if err != nil {
		d.log().Error("sweep_failed", "err", err)
	}
	if len(reopened) > 0 {
		d.log().Info("placements_reclaimed", "items", reopened)
	}
	return len(reopened) > 0
}

// answerProbe is the liveness half a peer's sweep consults: this node
// answers alive for exactly the placements it is serving right now. An
// unknown placement answers no, so a stale owner cannot veto its own
// reclaim (fleet's wire, unchanged).
func (d *Dispatcher) answerProbe(msg *nats.Msg) {
	d.mu.Lock()
	_, owned := d.served[string(msg.Data)]
	d.mu.Unlock()
	if owned {
		_ = msg.Respond([]byte("alive"))
		return
	}
	_ = msg.Respond([]byte("no"))
}

func (d *Dispatcher) acceptingWork() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.taking
}

// skipRace answers whether this node should leave an open placement
// alone: it is already serving it, it failed at it recently, or the
// persona is already busy here. All three are self-selection — the node
// simply does not claim (design 0003 §2), which needs no vocabulary.
func (d *Dispatcher) skipRace(itemID, persona string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.served[itemID]; ok {
		return true
	}
	if until, ok := d.backoff[itemID]; ok && time.Now().Before(until) {
		return true
	}
	for _, s := range d.served {
		if s.persona == persona {
			return true
		}
	}
	return false
}

// slotFree answers whether this placement may start an engine now, and
// why not when it may not. A duplicate persona is loud: two engines on
// one credential would race the same deterministic outcome ids.
func (d *Dispatcher) slotFree(itemID, persona string) (bool, string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.taking {
		return false, ""
	}
	if _, ok := d.served[itemID]; ok {
		return false, ""
	}
	if until, ok := d.backoff[itemID]; ok && time.Now().Before(until) {
		return false, ""
	}
	for _, s := range d.served {
		if s.persona == persona {
			return false, fmt.Sprintf("persona %s is already served by this node under another placement", persona)
		}
	}
	return true, ""
}

func (d *Dispatcher) take(itemID string, s *serve) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.taking {
		return false
	}
	if _, ok := d.served[itemID]; ok {
		return false
	}
	d.served[itemID] = s
	return true
}

func (d *Dispatcher) release(itemID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.served, itemID)
	d.backoff[itemID] = time.Now().Add(d.raceBackoff())
}

func (d *Dispatcher) backOff(itemID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.backoff[itemID] = time.Now().Add(d.raceBackoff())
}

func (d *Dispatcher) log() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.Default()
}

func (d *Dispatcher) reclaimBound() time.Duration {
	if d.Reclaim > 0 {
		return d.Reclaim
	}
	return defaultReclaim
}

// sweepEvery defaults to a quarter of the reclaim bound: a dead peer is
// nominated inside the window the operator configured, without a second
// knob to keep in step with the first.
func (d *Dispatcher) sweepEvery() time.Duration {
	if d.SweepEvery > 0 {
		return d.SweepEvery
	}
	if every := d.reclaimBound() / 4; every > minSweepEvery {
		return every
	}
	return minSweepEvery
}

func (d *Dispatcher) pollEvery() time.Duration {
	if d.PollEvery > 0 {
		return d.PollEvery
	}
	return defaultPollEvery
}

func (d *Dispatcher) raceBackoff() time.Duration {
	if d.RaceBackoff > 0 {
		return d.RaceBackoff
	}
	return defaultRaceBackoff
}

func (d *Dispatcher) drainTimeout() time.Duration {
	if d.DrainTimeout > 0 {
		return d.DrainTimeout
	}
	return defaultDrainTimeout
}

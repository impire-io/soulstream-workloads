package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nkeys"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/artifact"
	"github.com/impire-io/soulstream-workloads/backend/native"
	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/internal/natstest"
	"github.com/impire-io/soulstream-workloads/minter"
	"github.com/impire-io/soulstream-workloads/runner"
	"github.com/impire-io/soulstream-workloads/wrap"
)

// declaredRig is a hermetic realm for the 009 wake-engine scenarios: an
// owner who posts, and the declared agent's own client — the engine's whole
// standing (the runtime-side-reads decision).
type declaredRig struct {
	url   string
	owner *realm.Client
	agent *realm.Client
}

func startDeclaredRealm(t *testing.T) *declaredRig {
	t.Helper()
	url, shutdown := natstest.StartJetStream(t)
	t.Cleanup(shutdown)
	ctx := context.Background()
	connect := func(persona string) *realm.Client {
		nc, err := nats.Connect(url)
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		c, err := realm.NewClient(ctx, nc, realm.Config{Realm: "test-realm", Persona: persona})
		if err != nil {
			t.Fatalf("client %s: %v", persona, err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	owner := connect("owner")
	if _, err := owner.Provision(ctx); err != nil {
		t.Fatalf("provision: %v", err)
	}
	return &declaredRig{url: url, owner: owner, agent: connect("clerk")}
}

func (r *declaredRig) startTopic(t *testing.T, name string) string {
	t.Helper()
	h, err := topic.StartTopic(context.Background(), r.owner, topic.StartTopicInput{Name: name})
	if err != nil {
		t.Fatalf("start topic %s: %v", name, err)
	}
	return h.Path()
}

// buildDeclaredCfg parses a declaration document and maps it onto an engine
// config through the shipped path (DeclaredConfig over the agent's client).
func buildDeclaredCfg(t *testing.T, rig *declaredRig, declJSON string, tpl wrap.Template, scratch string) wrap.Config {
	t.Helper()
	d, err := declaration.Parse([]byte(declJSON))
	if err != nil {
		t.Fatalf("parse declaration: %v", err)
	}
	base := wrap.Config{Persona: "clerk", Template: tpl, Scratch: scratch,
		RunTimeout: 30 * time.Second, Retries: 1}
	cfg, err := wrap.DeclaredConfig(base, d, rig.agent)
	if err != nil {
		t.Fatalf("DeclaredConfig: %v", err)
	}
	return cfg
}

// runEngine starts the wake engine and returns an idempotent stop.
func runEngine(t *testing.T, client *realm.Client, cfg wrap.Config, invoke wrap.Invoker) (stop func()) {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	if testing.Verbose() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	w := &wrap.Wrapper{Config: cfg, Client: client, Invoke: invoke, Log: logger}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = w.Run(ctx) }()
	time.Sleep(500 * time.Millisecond) // subscriptions + reconcile attach
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("engine did not stop")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

// tickSeqs reads the surviving ticks on one TICKS subject: their stream
// sequences, oldest first (expired ticks are gone — that is the TTL bound).
func tickSeqs(t *testing.T, c *realm.Client, subject string) []uint64 {
	t.Helper()
	ctx := context.Background()
	stream, err := c.JetStream().Stream(ctx, realm.SystemStreamName)
	if err != nil {
		t.Fatalf("system stream: %v", err)
	}
	if _, err := stream.GetLastMsgForSubject(ctx, subject); err != nil {
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			return nil
		}
		t.Fatalf("probe ticks: %v", err)
	}
	cons, err := stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		FilterSubjects: []string{subject},
		DeliverPolicy:  jetstream.DeliverAllPolicy,
	})
	if err != nil {
		t.Fatalf("tick consumer: %v", err)
	}
	it, err := cons.Messages()
	if err != nil {
		t.Fatalf("tick messages: %v", err)
	}
	defer it.Stop()
	var seqs []uint64
	for {
		msg, err := it.Next()
		if err != nil {
			break
		}
		md, err := msg.Metadata()
		if err != nil {
			t.Fatalf("tick metadata: %v", err)
		}
		seqs = append(seqs, md.Sequence.Stream)
		if md.NumPending == 0 {
			break
		}
	}
	return seqs
}

func opSet(cs []topic.Contribution) map[string]bool {
	set := make(map[string]bool, len(cs))
	for _, c := range cs {
		set[c.OpID] = true
	}
	return set
}

func subjectDigest(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// SC-001 + SC-002 (design 0005 acceptance bar 1): four wake kinds fire from
// ONE declaration; every stream-backed kind answers exactly once across an
// engine restart — outcome count per trigger 1 in the stream, harness
// attempt count per trigger 1 on disk — and a subject publish while the
// engine is down is lost, as its delivery class declares.
func TestDeclaredFourKindsAnswerOnceAcrossRestart(t *testing.T) {
	rig := startDeclaredRealm(t)
	home := rig.startTopic(t, "clerk home")
	watched := rig.startTopic(t, "watched")
	mock := buildCmd(t, "github.com/impire-io/soulstream-workloads/cmd/harness-mock")
	scratch := t.TempDir() // shared across both engine runs: attempt dirs count
	const pingSubject = "declared.e2e.ping"

	declJSON := fmt.Sprintf(`{
		"role": "agent", "lifecycle": "service", "persona": "clerk",
		"topic": %q, "artifact": "file:///opt/agents/clerk",
		"wake": [
			{"kind": "mention"},
			{"kind": "topic", "path": %q},
			{"kind": "schedule", "name": "pulse", "pattern": "@every 1s", "ttl": "2m"},
			{"kind": "subject", "subject": %q}
		],
		"budget": {"max_hops": 4, "window": {"max": 100, "per": "10m"}}
	}`, home, watched, pingSubject)
	cfg := buildDeclaredCfg(t, rig, declJSON,
		mockTemplate(t, mock, "claude", "answered.", "ok", rig.url), scratch)

	// The backlog exists before anything runs: a mention and a watched turn.
	m1 := post(t, rig.owner, home, "@clerk backlog mention")
	w1 := post(t, rig.owner, watched, "a fresh turn in the watched topic")
	mentionOutcome := wrap.WakeOpID(m1, "clerk")
	topicOutcome := wrap.WakeOpID(w1, "clerk")

	tickSubject, err := realm.SystemTickSubject("clerk", "pulse")
	if err != nil {
		t.Fatal(err)
	}
	homeOps := func() map[string]bool { return opSet(turnsBy(t, rig.owner, home, "clerk")) }
	tickOutcomes := func(seqs []uint64) []string {
		out := make([]string, 0, len(seqs))
		for _, s := range seqs {
			out = append(out, wrap.WakeOpID(fmt.Sprintf("%d", s), "clerk"))
		}
		return out
	}
	waitFor := func(what string, pred func() bool) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for !pred() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	stop := runEngine(t, rig.agent, cfg, nil) // nil invoker = the real harness runner
	waitFor("the mention and topic outcomes", func() bool {
		return homeOps()[mentionOutcome] && opSet(turnsBy(t, rig.owner, watched, "clerk"))[topicOutcome]
	})

	// A live subject wake, and at least two schedule ticks answered.
	if err := rig.owner.Conn().Publish(pingSubject, []byte("ping-payload")); err != nil {
		t.Fatalf("publish subject: %v", err)
	}
	pingOutcome := wrap.WakeOpID(subjectDigest("ping-payload"), "clerk")
	waitFor("the subject outcome and two tick outcomes", func() bool {
		ops := homeOps()
		if !ops[pingOutcome] {
			return false
		}
		n := 0
		for _, id := range tickOutcomes(tickSeqs(t, rig.owner, tickSubject)) {
			if ops[id] {
				n++
			}
		}
		return n >= 2
	})

	// Down: a subject publish is lost (at-most-once, declared); ticks keep
	// accumulating — the server schedules whether or not anything runs.
	stop()
	if err := rig.owner.Conn().Publish(pingSubject, []byte("lost-payload")); err != nil {
		t.Fatalf("publish while down: %v", err)
	}
	lostOutcome := wrap.WakeOpID(subjectDigest("lost-payload"), "clerk")
	time.Sleep(2500 * time.Millisecond)
	downTicks := tickSeqs(t, rig.owner, tickSubject)

	// Restart mid-backlog: catch-up re-reads every source; the record is the
	// position, so nothing is answered twice and the down-time ticks are.
	runEngine(t, rig.agent, cfg, nil)
	waitFor("every down-time tick answered after restart", func() bool {
		ops := homeOps()
		for _, id := range tickOutcomes(downTicks) {
			if !ops[id] {
				return false
			}
		}
		return true
	})
	time.Sleep(1500 * time.Millisecond) // hold a beat: "exactly" means none extra

	// Stream count 1 per trigger: no duplicate outcome op anywhere, and every
	// home outcome is explained by exactly one trigger identity.
	for _, path := range []string{home, watched} {
		turns := turnsBy(t, rig.owner, path, "clerk")
		seen := map[string]int{}
		for _, c := range turns {
			seen[c.OpID]++
		}
		for op, n := range seen {
			if n != 1 {
				t.Fatalf("outcome %s appears %d times in %s — answered twice", op, n, path)
			}
		}
	}
	explained := map[string]bool{mentionOutcome: true, pingOutcome: true}
	for _, id := range tickOutcomes(tickSeqs(t, rig.owner, tickSubject)) {
		explained[id] = true
	}
	for op := range homeOps() {
		if !explained[op] {
			t.Fatalf("unexplained outcome %s on the home topic", op)
		}
	}
	if homeOps()[lostOutcome] {
		t.Fatal("a subject publish while the engine was down produced an outcome — at-most-once violated")
	}

	// Attempt count 1 per trigger: one run dir per answered wake, across both
	// engine runs (the outcome-existence pre-check gates before any invoke).
	required := []string{mentionOutcome, topicOutcome, pingOutcome}
	required = append(required, tickOutcomes(downTicks)...)
	for _, wakeID := range required {
		dirs, err := filepath.Glob(filepath.Join(scratch, wakeID+"-a*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(dirs) != 1 {
			t.Fatalf("wake %s has %d attempt dirs, want exactly 1", wakeID, len(dirs))
		}
	}
}

// SC-004: an op authored by the declared persona never wakes its own topic
// wake — zero invocations — while another author's op wakes it once.
func TestDeclaredTopicSelfExclusion(t *testing.T) {
	rig := startDeclaredRealm(t)
	watched := rig.startTopic(t, "watched")
	var invocations atomic.Int64
	declJSON := fmt.Sprintf(`{
		"role": "agent", "lifecycle": "service", "persona": "clerk",
		"topic": %q, "artifact": "file:///opt/agents/clerk",
		"wake": [{"kind": "topic", "path": %q}]
	}`, watched, watched)
	cfg := buildDeclaredCfg(t, rig, declJSON, wrap.Template{
		Command: []string{"/usr/bin/true"}, Prompt: "BODY={{BODY}}",
		Terminal: wrap.TerminalMap{TypeField: "type", TerminalValue: "result", TextField: "result"},
	}, t.TempDir())
	runEngine(t, rig.agent, cfg, func(_ context.Context, _ wrap.RunSpec) wrap.HarnessResult {
		invocations.Add(1)
		return wrap.HarnessResult{OK: true, Text: "noted."}
	})

	// The persona's own op: no wake, not ever.
	if _, err := topic.Open(rig.agent, watched).PostTurn(context.Background(), "my own note"); err != nil {
		t.Fatalf("agent post: %v", err)
	}
	time.Sleep(2500 * time.Millisecond)
	if n := invocations.Load(); n != 0 {
		t.Fatalf("the persona's own op invoked the harness %d times, want 0", n)
	}
	if got := turnsBy(t, rig.owner, watched, "clerk"); len(got) != 1 {
		t.Fatalf("clerk turns = %d, want only its own post", len(got))
	}

	// The control: another author's op wakes it exactly once.
	op := post(t, rig.owner, watched, "someone else speaks")
	waitTurns(t, rig.owner, watched, "clerk", 2)
	if invocations.Load() != 1 {
		t.Fatalf("invocations = %d, want exactly 1", invocations.Load())
	}
	if got := turnsBy(t, rig.owner, watched, "clerk"); got[1].OpID != wrap.WakeOpID(op, "clerk") {
		t.Fatalf("outcome op = %s, want the deterministic wake id", got[1].OpID)
	}
}

// SC-005: an uncooperative topic-wake cycle — two declared agents watching
// the same topic, every outcome waking the other — halts at the depth bound
// with loud, op-less refusals: the 0006 budget holds on the generalized
// engine (design 0005 §7's build requirement).
func TestDeclaredTopicCycleHaltsAtBudget(t *testing.T) {
	rig, path := startBudgetRealm(t, "owner", "agent-a", "agent-b")
	counter := &refusalCounter{}
	const maxHops = 4
	declared := func(c *wrap.Config) {
		c.Budget = wrap.Budget{MaxHops: maxHops, WindowMax: 100, WindowPer: time.Hour}
		c.Wakes = &wrap.WakeSet{Topics: []wrap.TopicWake{{Path: path}}}
		c.HomeTopic = path
	}
	stopA := runScriptWrapper(t, rig, "agent-a", declared, func(_, _ string) (string, bool) {
		return "noted, adding my view", true
	}, counter)
	defer stopA()
	stopB := runScriptWrapper(t, rig, "agent-b", declared, func(_, _ string) (string, bool) {
		return "noted, appending mine", true
	}, counter)
	defer stopB()

	rig.post(t, "owner", path, "kick the room")
	n, settled := rig.settle(t, path, 1500*time.Millisecond, 30*time.Second, "agent-a", "agent-b")

	if !settled {
		t.Fatalf("topic-wake cascade did not settle under the depth budget (%d agent turns)", n)
	}
	// Two chains root at the owner's post (one per agent), each capped at
	// maxHops provable outcomes.
	if n != 2*maxHops {
		t.Fatalf("agent turns = %d, want exactly 2×MaxHops = %d", n, 2*maxHops)
	}
	if counter.n.Load() < 1 {
		t.Fatal("expected at least one loud refusal")
	}
	for _, c := range rig.agentTurns(t, path, "agent-a", "agent-b") {
		if strings.Contains(c.Body, "could not answer") {
			t.Fatalf("a refusal posted testimony: %q", c.Body)
		}
	}
}

// SC-003: an instructions revision through ordinary ops reaches the very
// next wake's prompt on a running engine — no restart, no redeploy, nothing
// written outside the run scratch (the engine holds no copy at all).
func TestDeclaredInstructionsRevisionReachesNextWake(t *testing.T) {
	rig := startDeclaredRealm(t)
	home := rig.startTopic(t, "clerk home")
	soul := rig.startTopic(t, "clerk soul")
	ctx := context.Background()

	rootOp, err := topic.Open(rig.owner, soul).Attach(ctx, "clerk.md", "text/markdown",
		[]byte("v1: be brief"), "")
	if err != nil {
		t.Fatalf("attach v1: %v", err)
	}

	var mu sync.Mutex
	var prompts []string
	scratch := t.TempDir()
	declJSON := fmt.Sprintf(`{
		"role": "agent", "lifecycle": "service", "persona": "clerk",
		"topic": %q, "artifact": "file:///opt/agents/clerk",
		"instructions": {"topic": %q, "artefact": "clerk.md"},
		"wake": [{"kind": "mention"}]
	}`, home, soul)
	cfg := buildDeclaredCfg(t, rig, declJSON, wrap.Template{
		Command: []string{"/usr/bin/true"}, Prompt: "BODY={{BODY}}",
		Terminal: wrap.TerminalMap{TypeField: "type", TerminalValue: "result", TextField: "result"},
	}, scratch)
	runEngine(t, rig.agent, cfg, func(_ context.Context, spec wrap.RunSpec) wrap.HarnessResult {
		mu.Lock()
		prompts = append(prompts, spec.Prompt)
		mu.Unlock()
		return wrap.HarnessResult{OK: true, Text: "done."}
	})

	post(t, rig.owner, home, "@clerk first ask")
	waitTurns(t, rig.owner, home, "clerk", 1)
	mu.Lock()
	first := prompts[len(prompts)-1]
	mu.Unlock()
	if !strings.Contains(first, "v1: be brief") {
		t.Fatalf("first prompt lacks the instructions tip: %q", first)
	}

	// The revision is an ordinary op by an ordinary author — no redeploy.
	if _, err := topic.Open(rig.owner, soul).Revise(ctx, "clerk.md", "text/markdown",
		[]byte("v2: be thorough"), rootOp); err != nil {
		t.Fatalf("revise: %v", err)
	}

	post(t, rig.owner, home, "@clerk second ask")
	waitTurns(t, rig.owner, home, "clerk", 2)
	mu.Lock()
	second := prompts[len(prompts)-1]
	mu.Unlock()
	if !strings.Contains(second, "v2: be thorough") || strings.Contains(second, "v1: be brief") {
		t.Fatalf("second prompt does not carry the revision tip: %q", second)
	}

	// No durable copy: the engine wrote nothing — scratch is empty (the
	// script invoker never ran a process) and the tip travelled in memory.
	entries, err := os.ReadDir(scratch)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("scratch holds %d entries, want none — instructions must not persist", len(entries))
	}
}

// SC-006: the schedule substrate semantics the engine leans on, measured
// live — a re-published registration REPLACES (no double tick rate), purging
// the registration subject DEREGISTERS, and Nats-Schedule-TTL stamps every
// tick so the backlog is TTL-bounded.
func TestDeclaredScheduleReplacePurgeAndTTL(t *testing.T) {
	rig := startDeclaredRealm(t)
	ctx := context.Background()
	js := rig.owner.JetStream()

	register := func(name, pattern, ttl string) {
		t.Helper()
		sched, err := realm.SystemScheduleSubject("clerk", name)
		if err != nil {
			t.Fatal(err)
		}
		target, err := realm.SystemTickSubject("clerk", name)
		if err != nil {
			t.Fatal(err)
		}
		msg := &nats.Msg{Subject: sched, Header: nats.Header{
			jetstream.ScheduleHeader:       []string{pattern},
			jetstream.ScheduleTargetHeader: []string{target},
		}, Data: []byte("tick of " + name)}
		if ttl != "" {
			msg.Header.Set(jetstream.ScheduleTTLHeader, ttl)
		}
		if _, err := js.PublishMsg(ctx, msg); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	flashTicks, err := realm.SystemTickSubject("clerk", "flash")
	if err != nil {
		t.Fatal(err)
	}
	beatTicks, err := realm.SystemTickSubject("clerk", "beat")
	if err != nil {
		t.Fatal(err)
	}

	// TTL first (the stream is otherwise quiet, so stream-sequence arithmetic
	// counts the appended ticks): every tick carries the registration's TTL
	// and expires — the backlog is bounded, not accumulated.
	stream, err := js.Stream(ctx, realm.SystemStreamName)
	if err != nil {
		t.Fatal(err)
	}
	before := stream.CachedInfo().State.LastSeq
	register("flash", "@every 1s", "1s")
	time.Sleep(5 * time.Second)
	surviving := tickSeqs(t, rig.owner, flashTicks)
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	appended := int(info.State.LastSeq - before - 1) // minus the registration itself
	if appended < 3 {
		t.Fatalf("only %d ticks appended in 5s of @every 1s — the scheduler is not ticking", appended)
	}
	if len(surviving) > 2 {
		t.Fatalf("%d ticks survive a 1s TTL after 5s, want ≤2 — Nats-Schedule-TTL did not bound the backlog", len(surviving))
	}
	if len(surviving) >= appended {
		t.Fatalf("surviving %d of %d appended — nothing expired", len(surviving), appended)
	}
	// Freeze flash so the next measurements are clean.
	flashSched, _ := realm.SystemScheduleSubject("clerk", "flash")
	if err := stream.Purge(ctx, jetstream.WithPurgeSubject(flashSched)); err != nil {
		t.Fatalf("purge flash: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)

	// Replace: a slow schedule re-published fast ticks at the fast rate; a
	// second re-publish of the same fast pattern does not double it.
	register("beat", "@every 1h", "")
	time.Sleep(200 * time.Millisecond)
	register("beat", "@every 1s", "")
	time.Sleep(4 * time.Second)
	afterReplace := len(tickSeqs(t, rig.owner, beatTicks))
	if afterReplace < 2 {
		t.Fatalf("replaced schedule produced %d ticks in 4s, want ≥2 — re-publish did not replace", afterReplace)
	}
	register("beat", "@every 1s", "")
	time.Sleep(4 * time.Second)
	afterRepublish := len(tickSeqs(t, rig.owner, beatTicks))
	rate := afterRepublish - afterReplace
	if rate > 6 {
		t.Fatalf("%d ticks in 4s after re-publishing the same schedule — the rate doubled (re-publish must replace)", rate)
	}

	// Purge deregisters: no new ticks after the registration is purged.
	beatSched, _ := realm.SystemScheduleSubject("clerk", "beat")
	if err := stream.Purge(ctx, jetstream.WithPurgeSubject(beatSched)); err != nil {
		t.Fatalf("purge beat: %v", err)
	}
	time.Sleep(1200 * time.Millisecond) // let an in-flight tick land
	frozen := len(tickSeqs(t, rig.owner, beatTicks))
	time.Sleep(3 * time.Second)
	if got := len(tickSeqs(t, rig.owner, beatTicks)); got != frozen {
		t.Fatalf("ticks grew %d→%d after the registration was purged — purge did not deregister", frozen, got)
	}
}

// US4: a declaration whose artifact lives in the record boots end to end —
// the runner materialises the lineage tip into the run's scratch (digest
// checked, reaped with the run) — and a tampered store refuses the launch
// with the work item abandoned, never a silent serve.
func TestDeclaredRecordArtifactLaunches(t *testing.T) {
	rig := startDeclaredRealm(t)
	workTopic := rig.startTopic(t, "ops room")
	ctx := context.Background()

	marker := filepath.Join(t.TempDir(), "ran.txt")
	script := "#!/bin/sh\necho ran > \"$1\"\n"
	if _, err := topic.Open(rig.owner, workTopic).Attach(ctx, "clerk-bin",
		"application/octet-stream", []byte(script), ""); err != nil {
		t.Fatalf("attach artifact: %v", err)
	}

	acc, _ := nkeys.CreateAccount()
	accPub, _ := acc.PublicKey()
	seed, _ := acc.Seed()
	m, err := minter.NewSigningKeyMinter(seed, accPub, []string{rig.url})
	if err != nil {
		t.Fatalf("minter: %v", err)
	}
	scratchRoot := t.TempDir()
	r := &runner.Runner{
		Minter: m, Backend: native.New(), Realm: "test-realm",
		CredTTL: time.Hour, ScratchRoot: scratchRoot,
		Artifacts: &artifact.Resolver{Client: rig.owner},
	}
	d := declaration.Declaration{
		Role: declaration.RoleAgent, Lifecycle: declaration.LifecycleService,
		Persona: "clerk", Topic: workTopic,
		Artifact: "soulstream://" + workTopic + "/clerk-bin",
		Args:     []string{marker},
	}
	tc := topic.Open(rig.owner, workTopic)
	if err := r.Run(ctx, tc, d); err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || strings.TrimSpace(string(data)) != "ran" {
		t.Fatalf("the record artifact did not run: %q, %v", data, err)
	}
	// Never a durable copy: the run's scratch (and the materialised artifact
	// inside it) is reaped with the workload.
	entries, err := os.ReadDir(scratchRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("scratch root still holds %v — the resolved artifact must be reaped with the run", entries)
	}

	// Tamper the store: the digest check refuses and the work item ends
	// abandoned — work.open + work.abandon, no dangling claim.
	_, art, err := artifact.Fetch(ctx, rig.owner, workTopic, "clerk-bin")
	if err != nil {
		t.Fatalf("fetch for tamper: %v", err)
	}
	store, err := rig.owner.JetStream().ObjectStore(ctx, realm.ObjectBucket)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutBytes(ctx, art.Tip.Object, []byte("evil bytes")); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(ctx, tc, d); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("tampered launch = %v, want the digest refusal", err)
	}
	mt, err := topic.Open(rig.owner, workTopic).Materialise(ctx)
	if err != nil {
		t.Fatal(err)
	}
	abandoned := 0
	for _, w := range mt.WorkItems {
		for _, ev := range w.Timeline {
			if ev.Kind == "abandon" {
				abandoned++
			}
		}
	}
	if abandoned != 1 {
		t.Fatalf("abandon events = %d, want exactly the refused launch's", abandoned)
	}
}

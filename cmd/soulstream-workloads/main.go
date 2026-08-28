// Command soulstream-workloads launches workloads onto a node:
// `workload start <declaration-file>` runs one agent or tool (M1.1), and
// `dispatcher serve` runs the standing serve loop (specs/011) that makes
// submit-and-forget real — the fleet's answer to the daemon that once
// lived here as `waker serve`, cut the day it landed with its reversal
// condition recorded in design 0004 §9 and fired by design 0007. The
// personal agent wrapper is still its own command, soulstream-wrap
// (specs/006).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/impire-io/soulstream-core/realm"
	"github.com/impire-io/soulstream-core/topic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/impire-io/soulstream-workloads/backend"
	"github.com/impire-io/soulstream-workloads/backend/k8s"
	"github.com/impire-io/soulstream-workloads/backend/msb"
	"github.com/impire-io/soulstream-workloads/backend/native"
	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/dispatcher"
	"github.com/impire-io/soulstream-workloads/minter"
	"github.com/impire-io/soulstream-workloads/runner"
	"github.com/impire-io/soulstream-workloads/wrap"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "soulstream-workloads:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	switch {
	case len(args) == 3 && args[0] == "workload" && args[1] == "start":
		return runWorkloadStart(args[2])
	case len(args) == 2 && args[0] == "dispatcher" && args[1] == "serve":
		return runDispatcherServe()
	default:
		return fmt.Errorf("usage: soulstream-workloads workload start <declaration-file>\n" +
			"       soulstream-workloads dispatcher serve")
	}
}

func runWorkloadStart(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read declaration: %w", err)
	}
	d, err := declaration.Parse(data)
	if err != nil {
		return err
	}
	if err := d.Validate(); err != nil {
		return err
	}

	realmName := os.Getenv("SOULSTREAM_REALM")
	persona := os.Getenv("SOULSTREAM_PERSONA")
	signingSeed := os.Getenv("SOULSTREAM_REALM_SIGNING_KEY")
	rootAccount := os.Getenv("SOULSTREAM_ROOT_ACCOUNT")
	if realmName == "" || persona == "" || signingSeed == "" || rootAccount == "" {
		return fmt.Errorf("SOULSTREAM_REALM, SOULSTREAM_PERSONA, SOULSTREAM_REALM_SIGNING_KEY and SOULSTREAM_ROOT_ACCOUNT are all required")
	}

	ctx := context.Background()
	client, err := realm.Connect(ctx, realm.Config{
		ContextName: os.Getenv("SOULSTREAM_CONTEXT"),
		Realm:       realmName,
		Persona:     persona,
	})
	if err != nil {
		return fmt.Errorf("connect to realm: %w", err)
	}
	defer func() { _ = client.Close() }()

	m, err := minter.NewSigningKeyMinter([]byte(signingSeed), rootAccount, serversOf(client))
	if err != nil {
		return err
	}

	be, err := selectBackend(os.Getenv("SOULSTREAM_BACKEND"), os.Getenv("SOULSTREAM_MSB_IMAGE"))
	if err != nil {
		return err
	}

	r := &runner.Runner{
		Minter:      m,
		Backend:     be,
		Realm:       realmName,
		CredTTL:     24 * time.Hour,
		ScratchRoot: scratchRoot(),
	}

	rw, err := r.Launch(ctx, topic.Open(client, d.Topic), d)
	if err != nil {
		return err
	}

	// Serve until the workload exits on its own (agent/job) or we are signalled
	// to stop it (a persistent service).
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return rw.Serve(sigCtx)
}

// runDispatcherServe runs this node's standing serve loop (specs/011).
// Everything it reads is node-side configuration — none of it may appear
// in a declaration (constitution III) — and the whole of it is the
// deployment's, not an agent's.
func runDispatcherServe() error {
	realmName := os.Getenv("SOULSTREAM_REALM")
	node := os.Getenv("SOULSTREAM_PERSONA")
	placements := os.Getenv("SOULSTREAM_PLACEMENT_TOPIC")
	credsDir := os.Getenv("SOULSTREAM_AGENT_CREDS_DIR")
	if realmName == "" || node == "" || placements == "" || credsDir == "" {
		return fmt.Errorf("SOULSTREAM_REALM, SOULSTREAM_PERSONA, SOULSTREAM_PLACEMENT_TOPIC and SOULSTREAM_AGENT_CREDS_DIR are all required")
	}

	ctx := context.Background()
	nodeCfg := realmConfigFromEnv(realmName, node)
	nodeCfg.CredsFile = os.Getenv("SOULSTREAM_CREDS")
	client, err := realm.Connect(ctx, nodeCfg)
	if err != nil {
		return fmt.Errorf("connect to realm: %w", err)
	}
	defer func() { _ = client.Close() }()

	template, err := harnessTemplate(client)
	if err != nil {
		return err
	}

	var knobErrs []error
	knob := func(name string) time.Duration {
		v, err := envDuration(name)
		if err != nil {
			knobErrs = append(knobErrs, err)
		}
		return v
	}
	d := &dispatcher.Dispatcher{
		Node:         node,
		Client:       client,
		Placements:   placements,
		ConnectAgent: agentCredsConnector(realmName, credsDir),
		Engine: wrap.Config{
			Template: template,
			Scratch:  scratchRoot(),
		},
		Reclaim:      knob("SOULSTREAM_SWEEP_WINDOW"),
		SweepEvery:   knob("SOULSTREAM_SWEEP_EVERY"),
		ProbeTimeout: knob("SOULSTREAM_PROBE_TIMEOUT"),
		PollEvery:    knob("SOULSTREAM_POLL_EVERY"),
		Log:          slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	if len(knobErrs) > 0 {
		return errors.Join(knobErrs...)
	}

	// The stop ceremony is the operator's, chosen here deliberately (hq
	// design 0007 §6): a signal drains, so an in-flight harness failure
	// lands the agent's own self-report. A supervisor that wants crash
	// semantics — nothing posted, the successor re-serving on the
	// deterministic outcome id — kills the process instead.
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return d.Run(sigCtx)
}

// agentCredsConnector is the node's credential lane for served agents: a
// directory holding one creds file per persona. It is the one lane a node
// operator can wire with no new authority; a deployment that mints
// per-serve credentials (hq design 0007 §5) substitutes its own
// ConnectAgent instead — this command has no minting standing and no
// soulstream-identity dependency.
func agentCredsConnector(realmName, dir string) dispatcher.ConnectFunc {
	return func(ctx context.Context, persona string) (*realm.Client, error) {
		creds := filepath.Join(dir, persona+".creds")
		if _, err := os.Stat(creds); err != nil {
			return nil, fmt.Errorf("no credential for %s at %s: %w", persona, creds, err)
		}
		cfg := realmConfigFromEnv(realmName, persona)
		cfg.CredsFile = creds
		return realm.Connect(ctx, cfg)
	}
}

// harnessTemplate resolves the harness this node runs served agents on:
// a template file, or one of wrap's presets.
//
// The preset's tool door deliberately carries no persona and no
// credential. One dispatcher serves many personas from one template, so
// baking a lane identity into it would hand every agent's door the same
// credential; the per-agent capability credential is the product's to
// mint (hq design 0007 §5, spec 010's agent scope) and reaches the
// harness through an operator-authored template until it does.
func harnessTemplate(client *realm.Client) (wrap.Template, error) {
	if path := os.Getenv("SOULSTREAM_HARNESS_TEMPLATE"); path != "" {
		return wrap.LoadTemplate(path)
	}
	name := os.Getenv("SOULSTREAM_HARNESS")
	if name == "" {
		name = "claude"
	}
	return wrap.Preset(name, wrap.Lane{
		URL:   client.Conn().ConnectedUrl(),
		Realm: os.Getenv("SOULSTREAM_REALM"),
	})
}

// realmConfigFromEnv builds the connection shape shared by the node and
// every agent it serves. A URL and a saved context are alternatives, not
// a pair — a dispatcher whose whole configuration arrives in its
// environment has nothing to save a context from.
func realmConfigFromEnv(realmName, persona string) realm.Config {
	cfg := realm.Config{Realm: realmName, Persona: persona}
	if url := os.Getenv("SOULSTREAM_URL"); url != "" {
		cfg.URL = url
		return cfg
	}
	cfg.ContextName = os.Getenv("SOULSTREAM_CONTEXT")
	return cfg
}

// envDuration reads one optional liveness knob (design 0003 §6). Unset
// takes the dispatcher's default; set-but-unreadable fails the node's
// start, because a liveness bound silently falling back to a default is
// how a fleet gets a reclaim window nobody chose.
func envDuration(name string) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return 0, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s=%q is not a duration: %w", name, raw, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("%s=%q must be positive", name, raw)
	}
	return v, nil
}

// selectBackend maps the node-side backend choice (SOULSTREAM_BACKEND) to an
// isolation backend — the ONLY place isolation is chosen (constitution III;
// the declaration cannot name a backend, its parser rejects unknown fields).
// An unrecognised value fails loud before any op is published (FR-001).
func selectBackend(name, msbImage string) (backend.Backend, error) {
	switch name {
	case "", "native":
		return native.New(), nil
	case "msb":
		return &msb.Backend{Image: msbImage}, nil
	case "k8s":
		return k8sBackendFromEnv()
	default:
		return nil, fmt.Errorf("SOULSTREAM_BACKEND %q is not a known backend (native, msb, k8s)", name)
	}
}

// k8sBackendFromEnv builds the Kubernetes backend from SOULSTREAM_K8S_* node
// configuration. The registry is required (specs/004 FR/T017); config errors
// fail here, loud, before any op is published. Cluster access resolves via
// client-go's standard kubeconfig loading rules, with SOULSTREAM_K8S_CONTEXT
// selecting a non-default context.
func k8sBackendFromEnv() (backend.Backend, error) {
	registry := os.Getenv("SOULSTREAM_K8S_REGISTRY")
	if registry == "" {
		return nil, fmt.Errorf("SOULSTREAM_K8S_REGISTRY is required for the k8s backend (OCI repository prefix for per-run artifact images)")
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{CurrentContext: os.Getenv("SOULSTREAM_K8S_CONTEXT")}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("k8s backend: resolve kubeconfig: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("k8s backend: build client: %w", err)
	}
	return &k8s.Backend{
		Client:    cs,
		Namespace: os.Getenv("SOULSTREAM_K8S_NAMESPACE"),
		Registry:  registry,
		BaseImage: os.Getenv("SOULSTREAM_K8S_BASE_IMAGE"),
		HostAlias: os.Getenv("SOULSTREAM_K8S_HOST_ALIAS"),
	}, nil
}

// serversOf returns the realm's NATS server URLs, minted into workload creds so
// the workload knows where to connect. Falls back to the connected URL.
func serversOf(c *realm.Client) []string {
	if s := c.Conn().Servers(); len(s) > 0 {
		return s
	}
	if u := c.Conn().ConnectedUrl(); u != "" {
		return []string{u}
	}
	return nil
}

func scratchRoot() string {
	if d := os.Getenv("SOULSTREAM_SCRATCH"); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "soulstream-workloads")
}

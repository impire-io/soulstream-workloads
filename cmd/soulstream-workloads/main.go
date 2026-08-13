// Command soulstream-workloads launches workloads onto a node. M1.1: a single
// `soulstream-workloads workload start <declaration-file>` that runs one agent.
package main

import (
	"context"
	"fmt"
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
	"github.com/impire-io/soulstream-workloads/minter"
	"github.com/impire-io/soulstream-workloads/runner"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "soulstream-workloads:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 3 || args[0] != "workload" || args[1] != "start" {
		return fmt.Errorf("usage: soulstream-workloads workload start <declaration-file>")
	}

	data, err := os.ReadFile(args[2])
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

// Package natstest is a test-only helper that runs an in-process NATS server
// with JetStream enabled, so integration tests need no external server. It is
// under internal/ because it is not part of soulstream-workloads's public surface.
package natstest

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

// Option adjusts the in-process server's options before start. Defaults stay
// loopback-bound; e2e suites whose workloads run behind a wall (microVM
// guest, Kubernetes pod) bind 0.0.0.0 so the server is reachable through the
// backend's host alias (M2.1, T015).
type Option func(*server.Options)

// WithBindAddress binds the server to addr instead of 127.0.0.1.
func WithBindAddress(addr string) Option {
	return func(o *server.Options) { o.Host = addr }
}

// StartJetStream starts an in-process NATS server with JetStream enabled, backed
// by a per-test temporary store directory, and returns its client URL together
// with a cleanup function.
func StartJetStream(t *testing.T, options ...Option) (url string, cleanup func()) {
	t.Helper()

	opts := &server.Options{
		JetStream: true,
		StoreDir:  t.TempDir(),
		Host:      "127.0.0.1",
		Port:      -1,
		NoLog:     true,
		NoSigs:    true,
	}
	for _, o := range options {
		o(opts)
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("natstest: new server: %v", err)
	}

	go ns.Start()

	if !ns.ReadyForConnections(10 * time.Second) {
		ns.Shutdown()
		t.Fatal("natstest: server not ready for connections")
	}

	return ns.ClientURL(), ns.Shutdown
}

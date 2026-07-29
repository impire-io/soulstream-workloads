// Command scope-probe is the SC-004 reference workload: it verifies, from
// wherever it runs (native process, microVM, pod — the server cannot tell,
// which is the point), that its minted credential is enforced by the realm.
// It honors the SOULREALM_* env contract, connects with its credential,
// publishes in-scope on the transient SOULREALM.SVC.> namespace (allowed for
// the agent role, never captured by any stored stream), then publishes
// out-of-scope and requires the server's permissions violation.
//
// Exit codes:
//
//	0 — in-scope allowed AND out-of-scope denied (scope enforcement holds)
//	1 — could not connect
//	2 — the in-scope publish was denied
//	3 — the out-of-scope publish was NOT denied
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulrealm/backend/native"
)

const (
	inScopeSubject    = "SOULREALM.SVC.probe-ping"
	outOfScopeSubject = "SOULREALM.SCOPE.DENIED"
)

func main() {
	violations := make(chan error, 8)
	nc, err := nats.Connect(os.Getenv(native.EnvNatsServers),
		nats.UserCredentials(os.Getenv(native.EnvCredsFile)),
		nats.Timeout(15*time.Second),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, e error) { violations <- e }),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scope-probe: connect: %v\n", err)
		os.Exit(1)
	}
	defer nc.Close()

	// In scope: must pass without a violation.
	if err := nc.Publish(inScopeSubject, []byte("ping")); err != nil {
		fmt.Fprintf(os.Stderr, "scope-probe: in-scope publish: %v\n", err)
		os.Exit(2)
	}
	if err := nc.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "scope-probe: flush: %v\n", err)
		os.Exit(2)
	}
	select {
	case e := <-violations:
		fmt.Fprintf(os.Stderr, "scope-probe: in-scope publish denied: %v\n", e)
		os.Exit(2)
	case <-time.After(2 * time.Second):
	}

	// Out of scope: the server must deny it.
	if err := nc.Publish(outOfScopeSubject, []byte("nope")); err != nil {
		fmt.Fprintf(os.Stderr, "scope-probe: out-of-scope publish call: %v\n", err)
		os.Exit(3)
	}
	if err := nc.Flush(); err != nil {
		// A server may cut the connection on a violation; that IS a denial.
		fmt.Println("scope-probe: out-of-scope flush errored (connection cut) — denied")
		os.Exit(0)
	}
	select {
	case e := <-violations:
		if strings.Contains(strings.ToLower(e.Error()), "permission") {
			fmt.Println("scope-probe: out-of-scope publish denied — scope enforcement holds")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "scope-probe: unexpected async error: %v\n", e)
		os.Exit(3)
	case <-time.After(8 * time.Second):
		fmt.Fprintln(os.Stderr, "scope-probe: out-of-scope publish was NOT denied")
		os.Exit(3)
	}
}

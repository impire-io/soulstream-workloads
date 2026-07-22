package natstest

import (
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nkeys"
)

// OperatorServer is an in-process operator-mode NATS server plus the account
// signing material a minter needs to issue users the server will trust. Unlike
// StartJetStream's open server, this one ENFORCES user JWT permissions — so it
// can prove a scoped credential is denied outside its scope (SC-003).
type OperatorServer struct {
	URL                string
	AccountSigningSeed []byte // an account signing key seed the server trusts
	RootAccountKey     string // the account's public identity key
	Shutdown           func()
}

// StartOperator brings up an operator-mode server: one operator, one realm
// account with a signing key, and a memory resolver that trusts them. A user
// JWT signed by the account signing key (IssuerAccount = the account) is
// authenticated and its pub/sub permissions enforced.
func StartOperator(t *testing.T) OperatorServer {
	t.Helper()

	operator, err := nkeys.CreateOperator()
	if err != nil {
		t.Fatalf("natstest: operator key: %v", err)
	}
	opub, _ := operator.PublicKey()

	// The realm account and a signing key added to it.
	account, _ := nkeys.CreateAccount()
	apub, _ := account.PublicKey()
	signing, _ := nkeys.CreateAccount()
	spub, _ := signing.PublicKey()
	sseed, _ := signing.Seed()

	ac := jwt.NewAccountClaims(apub)
	ac.Name = "realm"
	ac.SigningKeys.Add(spub)
	ajwt, err := ac.Encode(operator)
	if err != nil {
		t.Fatalf("natstest: account jwt: %v", err)
	}

	// A system account, which operator mode expects.
	sysAcc, _ := nkeys.CreateAccount()
	syspub, _ := sysAcc.PublicKey()
	sysClaims := jwt.NewAccountClaims(syspub)
	sysClaims.Name = "SYS"
	sysjwt, err := sysClaims.Encode(operator)
	if err != nil {
		t.Fatalf("natstest: sys account jwt: %v", err)
	}

	res := &server.MemAccResolver{}
	if err := res.Store(apub, ajwt); err != nil {
		t.Fatalf("natstest: store account: %v", err)
	}
	if err := res.Store(syspub, sysjwt); err != nil {
		t.Fatalf("natstest: store sys account: %v", err)
	}

	opts := &server.Options{
		Host:            "127.0.0.1",
		Port:            -1,
		NoLog:           true,
		NoSigs:          true,
		TrustedKeys:     []string{opub},
		AccountResolver: res,
		SystemAccount:   syspub,
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("natstest: new operator server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		ns.Shutdown()
		t.Fatal("natstest: operator server not ready")
	}

	return OperatorServer{
		URL:                ns.ClientURL(),
		AccountSigningSeed: sseed,
		RootAccountKey:     apub,
		Shutdown:           ns.Shutdown,
	}
}

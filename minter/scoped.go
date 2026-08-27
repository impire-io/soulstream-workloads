package minter

import (
	"fmt"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// ScopedSigningKeyMinter mints workload users signed by a SCOPED account
// signing key: the minted JWT carries NO permissions of its own — the
// account JWT's scope template on this key, expanded server-side with the
// mint's tags, is the entire policy (hq design 0005 §5). The identity
// plane's D28 mint.ephemeral produces the same claim shape through an op;
// this minter produces it locally from a deployment-held role seed, for
// deployments that custody the seed themselves. It refuses a
// capability-less scope: the plain SigningKeyMinter is that lane — and the
// two lanes cannot share a key, because a scoped signing key rejects
// permission-carrying users (journey 0010) just as a plain one enforces
// nothing.
type ScopedSigningKeyMinter struct {
	roleSigningSeed []byte
	rootAccountKey  string
	natsServers     []string
}

// NewScopedSigningKeyMinter validates the signing material and returns the
// minter. roleSigningSeed is the seed of the scoped role signing key; the
// account JWT must endorse its public key with the capability scope
// template — a mis-endorsed key yields users the server rejects at
// connection, which is the verifier of record.
func NewScopedSigningKeyMinter(roleSigningSeed []byte, rootAccountKey string, natsServers []string) (*ScopedSigningKeyMinter, error) {
	kp, err := nkeys.FromSeed(roleSigningSeed)
	if err != nil {
		return nil, fmt.Errorf("minter: invalid role signing seed: %w", err)
	}
	if _, err := kp.PublicKey(); err != nil {
		return nil, fmt.Errorf("minter: role signing seed unusable: %w", err)
	}
	if !nkeys.IsValidPublicAccountKey(rootAccountKey) {
		return nil, fmt.Errorf("minter: %q is not a valid account public key", rootAccountKey)
	}
	if len(natsServers) == 0 {
		return nil, fmt.Errorf("minter: at least one NATS server is required")
	}
	return &ScopedSigningKeyMinter{
		roleSigningSeed: roleSigningSeed,
		rootAccountKey:  rootAccountKey,
		natsServers:     natsServers,
	}, nil
}

// Mint issues a fresh permission-less scoped user for the capability scope:
// name = persona, tags = the scope's mint tags, TTL required. The server
// expands the role key's template with the tags at connection time.
func (m *ScopedSigningKeyMinter) Mint(s Scope, ttl time.Duration) (PersonaScopedCredential, error) {
	if s.Capabilities == nil {
		return PersonaScopedCredential{}, fmt.Errorf("minter: a scoped mint needs capabilities (the plain SigningKeyMinter is the capability-less lane)")
	}
	if s.Persona == "" || s.Topic == "" {
		return PersonaScopedCredential{}, fmt.Errorf("minter: scope needs both persona and topic")
	}
	if ttl <= 0 {
		return PersonaScopedCredential{}, fmt.Errorf("minter: ttl must be positive")
	}
	tags, err := s.MintTags()
	if err != nil {
		return PersonaScopedCredential{}, err
	}

	ukp, err := nkeys.CreateUser()
	if err != nil {
		return PersonaScopedCredential{}, fmt.Errorf("minter: create user key: %w", err)
	}
	upub, err := ukp.PublicKey()
	if err != nil {
		return PersonaScopedCredential{}, fmt.Errorf("minter: user public key: %w", err)
	}
	useed, err := ukp.Seed()
	if err != nil {
		return PersonaScopedCredential{}, fmt.Errorf("minter: user seed: %w", err)
	}

	exp := time.Now().Add(ttl)
	uc := jwt.NewUserClaims(upub)
	uc.Name = s.Persona
	uc.IssuerAccount = m.rootAccountKey
	uc.Expires = exp.Unix()
	uc.SetScoped(true)
	uc.Tags.Add(tags...)

	vr := &jwt.ValidationResults{}
	uc.Validate(vr)
	if vr.IsBlocking(true) {
		return PersonaScopedCredential{}, fmt.Errorf("minter: invalid scoped user claims: %v", vr.Errors())
	}

	rkp, err := nkeys.FromSeed(m.roleSigningSeed)
	if err != nil {
		return PersonaScopedCredential{}, fmt.Errorf("minter: load role signing key: %w", err)
	}
	token, err := uc.Encode(rkp)
	if err != nil {
		return PersonaScopedCredential{}, fmt.Errorf("minter: encode scoped user JWT: %w", err)
	}

	return PersonaScopedCredential{
		UserJWT:     token,
		UserSeed:    useed,
		NatsServers: m.natsServers,
		Persona:     s.Persona,
		Expires:     exp,
	}, nil
}

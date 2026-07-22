package minter

import (
	"fmt"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// PersonaScopedCredential is a freshly minted, scoped NATS user for one
// workload. The seed is a secret: it is delivered to the workload and never
// logged or published (constitution I/II).
type PersonaScopedCredential struct {
	UserJWT     string
	UserSeed    []byte
	NatsServers []string
	Persona     string
	Expires     time.Time
}

// Minter issues per-workload scoped credentials. It is the single seam through
// which soulrealm obtains signing authority (design 0001 §4): the default holds
// a soulrealm-side key; an external authority could implement this later without
// changing any workload contract.
type Minter interface {
	Mint(s Scope, ttl time.Duration) (PersonaScopedCredential, error)
}

// SigningKeyMinter mints users signed by a realm-account signing key that the
// realm's NATS operator trusts. Soulrealm holds the signing seed (episode 0003:
// soulstream-only scope).
type SigningKeyMinter struct {
	accountSigningSeed []byte
	rootAccountKey     string
	natsServers        []string
}

// NewSigningKeyMinter validates the signing material and returns a minter.
// accountSigningSeed is the account signing key seed ("SA..."); rootAccountKey
// is the account's public identity key ("A..."), bound into each user as
// IssuerAccount so the server can resolve and trust it.
func NewSigningKeyMinter(accountSigningSeed []byte, rootAccountKey string, natsServers []string) (*SigningKeyMinter, error) {
	kp, err := nkeys.FromSeed(accountSigningSeed)
	if err != nil {
		return nil, fmt.Errorf("minter: invalid account signing seed: %w", err)
	}
	if _, err := kp.PublicKey(); err != nil {
		return nil, fmt.Errorf("minter: account signing seed unusable: %w", err)
	}
	if !nkeys.IsValidPublicAccountKey(rootAccountKey) {
		return nil, fmt.Errorf("minter: %q is not a valid account public key", rootAccountKey)
	}
	if len(natsServers) == 0 {
		return nil, fmt.Errorf("minter: at least one NATS server is required")
	}
	return &SigningKeyMinter{
		accountSigningSeed: accountSigningSeed,
		rootAccountKey:     rootAccountKey,
		natsServers:        natsServers,
	}, nil
}

// Mint issues a fresh user scoped to the persona/topic for ttl.
func (m *SigningKeyMinter) Mint(s Scope, ttl time.Duration) (PersonaScopedCredential, error) {
	if s.Persona == "" || s.Topic == "" {
		return PersonaScopedCredential{}, fmt.Errorf("minter: scope needs both persona and topic")
	}
	if ttl <= 0 {
		return PersonaScopedCredential{}, fmt.Errorf("minter: ttl must be positive")
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

	perms := s.PermissionSet()
	exp := time.Now().Add(ttl)

	uc := jwt.NewUserClaims(upub)
	uc.Name = s.Persona
	uc.IssuerAccount = m.rootAccountKey
	uc.Expires = exp.Unix()
	uc.Pub.Allow = jwt.StringList(perms.Pub)
	uc.Sub.Allow = jwt.StringList(perms.Sub)

	vr := &jwt.ValidationResults{}
	uc.Validate(vr)
	if vr.IsBlocking(true) {
		return PersonaScopedCredential{}, fmt.Errorf("minter: invalid user claims: %v", vr.Errors())
	}

	akp, err := nkeys.FromSeed(m.accountSigningSeed)
	if err != nil {
		return PersonaScopedCredential{}, fmt.Errorf("minter: load signing key: %w", err)
	}
	token, err := uc.Encode(akp)
	if err != nil {
		return PersonaScopedCredential{}, fmt.Errorf("minter: encode user JWT: %w", err)
	}

	return PersonaScopedCredential{
		UserJWT:     token,
		UserSeed:    useed,
		NatsServers: m.natsServers,
		Persona:     s.Persona,
		Expires:     exp,
	}, nil
}

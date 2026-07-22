# Contracts — interfaces & declaration schema

Go interface sketches (signatures, not final code) and the declaration wire
form. These are the seams the rest of the runtime is built on; keeping them
small is constitution II/III in code.

## The workload declaration (wire form)

A single file, strict-decoded (unknown fields fail loud — SC-005):

```json
{
  "role": "agent",
  "lifecycle": "service",
  "persona": "researcher",
  "topic": "acme/q2-planning",
  "artifact": "file:///opt/agents/researcher",
  "args": ["--watch"]
}
```

No backend key exists in the schema; the node picks the backend
(constitution III). `declaration.Parse([]byte) (Declaration, error)` and
`Declaration.Validate() error` are **pure** (no NATS).

## Minter

Reimplements NEX's `CredVendor` *shape* against realm-semantic scope. The
claim-building half is pure; only `Mint` signs (needs the account key, no NATS).

```go
package minter

// Scope is the pure description of what a persona may touch on a topic.
// Built and tested with no server; see data-model PermissionSet.
type Scope struct {
    Persona string
    Topic   string
}

// PermissionSet turns a Scope into pub/sub allow-lists (PURE).
func (s Scope) PermissionSet() PermissionSet

// Minter issues a per-workload user scoped to a persona. Holds the realm
// account signing key + root account public key (soulrealm-held; the seam
// that could later delegate to an external authority — episode 0003).
type Minter interface {
    // Mint returns a fresh scoped user credential for the persona/topic.
    Mint(s Scope, ttl time.Duration) (PersonaScopedCredential, error)
}
```

`Mint`: create user nkey → build `jwt.UserClaims` with `PermissionSet()`,
`IssuerAccount` = realm account, `Name` = persona, `Expires` = now+ttl → sign
with the account signing key. Mirrors nex `internal/credentials/signing_key.go`
in shape, with `WorkloadClaims` replaced by the realm-semantic `PermissionSet`.

## Backend

The isolation seam (constitution III). `native` is the only implementation in
M1.1; Docker (M1.3) implements the same interface with the declaration
unchanged.

```go
package backend

// Handle is a running workload the runner can observe and stop.
type Handle interface {
    Wait() ExitStatus     // blocks; ExitStatus maps to work.done vs work.abandon
    Stop(context.Context) error
}

// Backend launches a workload artifact with an injected credential.
type Backend interface {
    // Start fetches the artifact, injects creds (D4: local env for native),
    // starts the workload, and returns a Handle. It does NOT touch soulstream —
    // lifecycle publication is the runner's job (keeps the backend swappable).
    Start(context.Context, LaunchSpec) (Handle, error)
}

type LaunchSpec struct {
    Artifact  string                    // resolved file:// path (M1.1)
    Args      []string
    Cred      minter.PersonaScopedCredential // injected into the child env
    ScratchDir string
}
```

**Key boundary**: the `Backend` emits no ops. The `runner` owns all soulstream
publication, so any backend is observable the same way (constitution V) and
none can leak a private control channel.

## Runner

Ties it together; owns the soulstream I/O. The lifecycle→op mapping it uses is
pure and tested without a server (see [`lifecycle-ops.md`](lifecycle-ops.md)).

```go
package runner

// Run executes one declaration to completion:
//   validate → open+claim the work item (runner persona) → mint workload cred
//   → backend.Start → on exit publish work.done|abandon → reap scratch/cred.
func (r *Runner) Run(ctx context.Context, d declaration.Declaration) error
```

`Runner` holds: a soulstream client for the **runner persona**, a `Minter`, and
a `Backend`. Failure before launch (bad persona, mint fails, artifact missing)
→ refuse, surface an observable error, publish nothing half-done (FR-008).

# Quickstart — Launch an agent

The end-to-end walk-through for M1.1, doubling as the acceptance script. It
describes the slice **once built**; the steps map 1:1 to the spec's success
criteria. Everything is soulstream + soulrealm only (episode 0003).

## Prerequisites

- Go 1.26, `nsc`, `nats-server` (operator mode), the `nats` CLI.
- A built `soulrealm` binary (`make build`) and a built agent artifact — for
  M1.1 the agent is a tiny program that connects with its injected creds and
  posts one turn via the soulstream client.

## 1. An operator-mode realm

Set up an operator, a **realm account** with a signing key, and a resolver the
server trusts (the shape of nex's `_examples/operator_mode`):

```sh
nsc add operator soulrealm-dev
nsc add account realm            # the realm's NATS account
nsc edit account realm --sk generate   # an account SIGNING KEY — soulrealm holds this
# start nats-server with the resolver pointed at these JWTs
```

Provision the soulstream realm on that account (soulstream's own tooling):

```sh
export SOULSTREAM_CONTEXT=soulrealm-dev SOULSTREAM_REALM=acme
soulstream provision          # SOULSTREAM stream + object store + persona directory
soulstream start "q2-planning"   # → prints the topic path, e.g. acme/q2-planning
```

## 2. Declare the agent

```json
// researcher.json
{
  "role": "agent",
  "lifecycle": "service",
  "persona": "researcher",
  "topic": "acme/q2-planning",
  "artifact": "file:///opt/agents/researcher"
}
```

## 3. Launch it

Soulrealm connects as its **runner** persona and holds the realm-account signing
key (from step 1) so it can mint:

```sh
export SOULREALM_REALM_SIGNING_KEY=...   # the account signing seed (SA...)
export SOULREALM_ROOT_ACCOUNT=...        # the account public key (A...)
soulrealm workload start ./researcher.json
```

What happens: validate → the runner publishes `work.open` then `work.claim` on
`acme/q2-planning` → mint a `researcher`-scoped credential → launch the artifact
natively with that credential in its env → the agent connects as `researcher`
and posts a turn.

## 4. Verify (the success criteria)

```sh
# SC-001: the agent's turn appears attributed to `researcher`
soulstream watch acme/q2-planning       # see a turn.post authored by researcher

# SC-002: lifecycle is on the topic; nothing private elsewhere
soulstream watch acme/q2-planning       # see work.open → work.claim in order
nats sub '>' &                          # audit: no soulrealm-private control subject

# SC-003: the minted cred is scoped
#   using the workload's creds, publishing an unrelated subject is DENIED:
nats --creds <workload.creds> pub SOMETHING.ELSE hi   # → permissions violation

# SC-004: kill → work.abandon + reap
kill <agent pid>
soulstream watch acme/q2-planning       # see work.abandon(reason=signal)
#   scratch dir gone, temp creds file gone

# SC-005: backend-agnostic declaration
#   adding a "backend": "docker" key and re-running is REJECTED at validation
```

## What this proves

The four load-bearing articles end to end on the smallest real workload:
identity is minted and scoped (II), the agent participates as a first-class
persona, its life is legible ops on the one control plane (V), soulrealm stored
nothing durable (I), and the declaration never named its backend (III).

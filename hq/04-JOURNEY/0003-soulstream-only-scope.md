# Episode 0003 — Soulstream-only scope: the platform waits (2026-07-22)

A scope boundary, set while drafting the first feature spec: **soulrealm
depends on soulstream and nothing else.** It provisions no part of the wider
Impire platform and takes no dependency on its services — identity, tenancy,
vault — for now. `[judgment]`, the maintainer's call.

**What changed.** The M1.1 spec and design 0001 had named **impire-tenants**
(the platform's sole writer of account signing keys) as the production path for
credential minting, with soulrealm holding a key only "for the first slice."
That framing is dropped. soulrealm holds the realm-account signing key,
full stop, for this and every near-term slice. Everything the runtime needs —
realm, topics, personas, object store, and the account it mints credentials
under — is soulstream's surface plus a soulrealm-held signing key
(dev: provisioned with `nsc`).

**What is preserved.** The minter stays a **seam** (design 0001 §4): an external
signing authority could take over later without changing the workload contract.
The difference is that no such external dependency is *designed in* now — the
seam is kept for optionality, not built toward a named platform service.

**Why.** Two moving projects are enough. Coupling soulrealm's first runnable
slice to impire-tenants (itself fronting a control plane) and the impire-vault
rebuild would gate soulrealm's progress on platform work that is out of scope
for proving the runtime. Keeping the dependency surface to soulstream alone
keeps the first slice self-contained and the bet — op-log as the single control
plane — testable without platform entanglement.

**Refuted / reversed.** The "soulrealm-held key is a temporary stand-in for
impire-tenants" framing in the draft spec and design 0001 §4. It is not a
stand-in; it is the design, until the platform is deliberately brought in.

Reversal condition: revisit platform integration (signing authority delegated
to an external Impire service) only when soulrealm has a working single-node
runtime *and* there is a concrete multi-realm/multi-tenant need that a
soulrealm-held key cannot serve. Until both hold, the dependency stays
soulstream-only.

Trail: `hq/02-DESIGN/0001-soulrealm-runtime.md` (§1 dependency scope, §4 Trust);
`specs/001-launch-an-agent/spec.md` (FR-009, Assumptions);
`hq/03-IMPLEMENTATION/roadmap.md`. Commits <pending>.

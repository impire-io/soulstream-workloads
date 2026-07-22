# JOURNEY — nex-runtime-substrate

Investigation log. Append entries as the work happens; conclusions graduate
into an `04-JOURNEY/` episode via `/research-graduate`.

## 2026-07-22 — topic opened

Opened alongside the soulrealm genesis (episode 0001). Captured the founder's
framing (NEX as substrate; `agent`/`tool` as workload types) and the initial
desk research on NEX v0.4.1:

- NEX already has a **lifecycle** workload axis: services (long-lived) vs
  functions (short-lived/triggered). Native + Firecracker for services; WASM +
  JS for functions.
- Per-workload **scoped NATS credentials** are delivered by the substrate.
- Runtime is **pluggable via nexlets**; NEX complements (does not replace)
  container runtimes.
- **Naming risk:** "agent" is close to NEX's node-runtime term (nexlet, née
  agent). A soulrealm `agent` workload type would share a stack with NEX's own
  "agent" concept — a terminology hazard to resolve before it ships.

Working hypothesis recorded in README: `agent`/`tool` is a **role** axis
orthogonal to NEX's **lifecycle** axis, so it should live at the highest
non-invasive layer (persona metadata > convention over service/function >
custom nexlet type), not be forced into a single NEX workload type. Bars 1–4
pre-registered to test substrate fit and the correct layer.

## 2026-07-22 — spike 1: live node + source read (dev build 0.0.0)

Ran a real stack on this machine: `nats-server v2.12.4 -js`, `nex node up
--dev-mode` (node came up ready, one native runtime registered), plus a read
of the exact source (`~/Work/nex`, built today — the source for the installed
binary; canonical for this build, *not* the 0.4.1 blog). Findings, all
**[measured] from the running binary and its source** unless noted:

**Bar 3 (role vs lifecycle) — strongly supported, ahead of schedule.** The
`nex workload start` surface has *two independent* native axes: `--type`
(runtime, default `native`, pluggable) and `--lifecycle` (**service |
function | job** — three, not the blog's two). Neither is a "role" axis.
Soulrealm's `agent`/`tool` is orthogonal to *both*. The role distinction is
genuinely absent from NEX and is soulrealm's to define. [measured]

**Naming hazard — confirmed, harder than expected.** NEX's own word for a
pluggable runtime *is* "agent": `nex node up --agents=…`,
`--allow-agent-registration`, node log `agent registered name=go_exec
type=native`, and `node list` column "Running Agents". A soulrealm workload
role called `agent` collides head-on with NEX's node-runtime concept on the
same stack. Strong reason to pick a different word for the role (candidate:
keep `persona`/`worker`, or name roles `participant`/`capability`). [measured]

**Bar 1 (scoped identity) — mechanism confirmed, but the scope is
NEX-operational, not realm-semantic. This is the load-bearing finding.**
- NEX mints a **fresh, uniquely-keyed NATS user per workload**
  (`internal/credentials/signing_key.go`: `nkeys.CreateUser()` +
  `jwt.NewUserClaims`, signed by the node's signing key, `IssuerAccount =
  RootAccountKey`), delivered in the start request's **required**
  `workload_creds` field, with the env **xkey-encrypted** to the runtime.
  So soulrealm need not build credential *minting/issuance* — the node is the
  minter. The strong half of the fit is real. [measured]
- **But the default scope is tiny.** `WorkloadClaims(namespace, workloadId)`
  (`internal/credentials/vendor.go`) grants **Pub: `_INBOX.>` only** and Sub:
  its own `logs.<namespace>.>` + inbox. A stock workload **cannot publish
  `SOULSTREAM.TOPICS.OPS.>`** — it can only do request-reply. The permission
  templates are overridable package-level `var`s: a seam, but a source-level
  one, not a config surface.
- **And dev-mode gives no identity at all.** `FullAccessMinter`
  (`nocreds.go`, the `--dev-mode` path) returns connection data with *no JWT
  and no seed* — the workload connects anonymously and inherits the open
  server's permissions. Per-persona scoping requires **operator mode** (node
  with signing key + root account key against a NATS server in operator/JWT
  auth). The `_examples/operator_mode` (nsc) is the reference setup.

**Reframing for the verdict:** NEX owns *identity issuance* and *lifecycle*
cleanly; it does **not** own *realm-semantic authorization* (which persona may
write which soulstream subjects). Bar 1 as written — publish a soulstream op
under a distinct persona *without soulrealm minting/brokering* — does **not**
pass with stock NEX. The three live options, to decide at graduation:
  1. **Widen the permission template** (override `WorkloadClaims` to allow the
     realm's op subjects) — closest to "NEX does it," but a source-level seam.
  2. **Broker writes** — workload does request-reply to a soulrealm/soulstream
     responder that holds the write creds; keeps stock NEX, adds a hop and a
     trusted writer.
  3. **Soulrealm supplies its own minter/permission policy** as the node's
     credential strategy — the pluggable-minter seam exists
     (SigningKey/Nkey/FullAccess are interchangeable `Minter`s).

Option 3 looks most aligned with constitution II (identity is the realm's, not
a bolt-on) *and* keeps NEX unforked, because the minter is already an
interface. To confirm before deciding: does the node accept a custom minter
without a code change, or only via the built-in three? That is the sharpened
next step — spike 2, in operator mode.

**Backend orthogonality (Bar 2) — not yet exercised.** Only the native
runtime ran. Docker/Firecracker backend swap still to spike.

Spike infra (nats-server, nex node) was torn down after capture; it is
throwaway per the research method and re-stands with two commands.

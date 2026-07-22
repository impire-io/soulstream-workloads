# Is NEX the right substrate, and is `agent`/`tool` the right way to model workloads on it?

**State:** active
**Started:** 2026-07-22

## Abstract

Soulrealm needs a substrate that launches, supervises, and observes a realm's
workloads over NATS, with scoped credentials per workload and pluggable
isolation (native / Docker / Firecracker / Kubernetes). NEX (the [NATS
Execution Engine](https://github.com/synadia-io/nex)) is the obvious
candidate: it is NATS-native, gives every workload scoped NATS credentials,
and its runtime layer (*nexlets*) is explicitly pluggable — "bring your own
runtime by writing a pluggable nexlet." The founder's instinct is to model
soulrealm workloads as two kinds — `agent` (a long-lived persona that
participates in topics) and `tool` (a capability other workloads call) — and
possibly to realise them as **NEX workload types**. This topic decides both:
(a) is NEX the substrate, and (b) is `agent`/`tool` a real modelling axis, and
at *which layer* does it live?

## The question

Should soulrealm build on NEX as its execution substrate; and is `agent` vs
`tool` the right decomposition of realm workloads — and if so, is it a NEX
**workload type** (a custom nexlet), a **convention layered over** NEX's
existing service/function workloads, or **soulstream persona metadata** with
NEX unaware of it?

## Context gathered (2026-07-22, [mechanism-argument] unless tagged)

From current NEX material (v0.4.1, March 2026; pre-1.0):

- NEX already splits workloads on a **lifecycle** axis: **services**
  (long-lived, native/Firecracker binaries) vs **functions** (short-lived,
  triggered on demand; WASM or JS).
- Every workload gets **scoped NATS credentials** at launch — pub/sub,
  request-reply, JetStream immediately available. This is exactly
  constitution II (one identity, no privileged tier) delivered by the
  substrate.
- The runtime is **pluggable via nexlets** ("bring your own runtime"); NEX
  "complements, does not replace" container runtimes (Podman, Kubernetes) —
  this is exactly the constitution III backend-orthogonality seam, already
  present.
- **Naming collision risk:** in NEX's own vocabulary a *nexlet* (formerly
  "agent") is the node-side runtime component. Calling a soulrealm workload an
  "agent" risks colliding with NEX's node-runtime term on the same stack.

**Working observation, not yet a decision:** `agent`/`tool` and
`service`/`function` are *different axes*. `agent`/`tool` is a **role/contract**
distinction (what a workload is *to the realm* — a participating persona vs a
called capability); `service`/`function` is a **lifecycle** distinction (how
NEX schedules it). An agent is almost always a NEX service. A tool could be
*either* a long-lived service (an MCP server holding request-reply on a
subject) *or* a short-lived function (spun up per call). Collapsing the two
axes into one would force every tool to a single lifecycle — the suspected
design error this topic exists to confirm or refute.

## Spike 1 findings (2026-07-22 — see JOURNEY.md for detail)

Live `nex node up` + source read of the exact dev build (`~/Work/nex`).
All **[measured]**:

- **Role is orthogonal to NEX's axes (Bar 3 leaning PASS).** `nex workload
  start` exposes `--type` (runtime; pluggable) *and* `--lifecycle` (service |
  function | **job**) — two native axes, neither of them "role." `agent`/`tool`
  is soulrealm's to define.
- **Naming hazard confirmed.** NEX already uses "agent" for its pluggable
  node runtime (`--agents`, `--allow-agent-registration`, `node list` →
  "Running Agents"). A soulrealm role named `agent` collides. Rename the role.
- **Identity issuance: NEX delivers it (Bar 1 mechanism PASS).** The node
  mints a fresh uniquely-keyed NATS user per workload, signed under a root
  account, delivered via required `workload_creds` in an xkey-encrypted env.
  Soulrealm need not build minting.
- **But authorization scope is NEX-operational, not realm-semantic (Bar 1 as
  written: FAIL vs stock).** Default `WorkloadClaims` allows Pub `_INBOX.>`
  only — a stock workload cannot publish soulstream op subjects. And
  `--dev-mode` issues *no* creds (anonymous/full access); real scoping needs
  operator mode. The minter is a pluggable interface
  (SigningKey/Nkey/FullAccess), so the sharpened question is whether soulrealm
  can supply its own minter/permission policy without forking NEX.

## Pre-registered bars

- **Bar 1 — NEX carries the identity model.** A spike launches two workloads
  on a real NEX node; each receives scoped NATS credentials and can publish a
  soulstream op attributed to a distinct persona *without* soulrealm minting
  or brokering credentials itself. PASS if the substrate delivers per-workload
  scoped identity end-to-end; FAIL if soulrealm must run its own credential
  tier to get per-persona attribution (which would strain constitution II).
  — *Spike 1 result (partial):* issuance PASSES (node mints per-workload
  users); but the default scope forbids publishing realm subjects and dev-mode
  issues nothing, so "publish a soulstream op with no soulrealm-side credential
  policy" FAILS against stock NEX. **Successor sub-question (spike 2, operator
  mode):** can soulrealm register a custom minter / permission template as the
  node's credential strategy without forking NEX? If yes, Bar 1 recovers to
  PASS via the pluggable-minter seam; if no, per-realm authorization is
  soulrealm's tier to own (broker or sidecar).

- **Bar 2 — Backend orthogonality is real, not aspirational.** The same
  workload declaration runs unchanged under at least two isolation backends
  (native process + one of Docker/Firecracker) via nexlet selection, with no
  change to the workload's own manifest. PASS if the backend is a node-side
  choice invisible to the declaration; FAIL if switching backends requires
  editing the workload contract (which would break constitution III).

- **Bar 3 — The role axis is orthogonal to NEX's lifecycle axis.** Enumerate
  the concrete workloads soulrealm must run in year one (a conversational
  agent; a persistent MCP-style tool; an on-demand code/exec tool; a
  scheduled/triggered job). Map each onto {agent,tool} × {service,function}.
  PASS if the four-cell mapping is populated and coherent — i.e. `tool`
  legitimately spans both lifecycles — confirming role must NOT be a lifecycle
  synonym. FAIL if every real workload collapses to one diagonal, meaning the
  distinction is cosmetic and NEX's native service/function axis already
  suffices.

- **Bar 4 — Lowest layer that works.** For whichever of the three
  realisations survives Bars 1–3 (custom nexlet workload *type* / convention
  over service+function / soulstream persona metadata), a spike demonstrates
  an agent and a tool launched, discovered, and called end-to-end. PASS if the
  chosen layer needs no fork of NEX and no privileged soulrealm tier; FAIL if
  it requires either. Prefer the *highest* (least invasive) layer that passes:
  metadata > convention > custom nexlet, in that order, unless a lower layer
  is forced by a failing higher one.

## Reversal condition

Adopt NEX as substrate only while these hold; revert (to a
container-runtime-direct design, or an own thin supervisor) if any is later
observed: NEX cannot surface a workload's lifecycle as ops a persona can
follow (breaks constitution V); NEX's pre-1.0 API churns hard enough that
tracking it costs more than a direct-to-backend supervisor would; or the
pluggable-nexlet story proves to require a NEX fork to run our backends
(breaks constitution III cheaply). Reconsider the `agent`/`tool` split itself
if Bar 3 collapses to a diagonal — in which case model workloads on NEX's
native service/function axis and carry role as pure persona metadata.

## Verdict

**Graduated 2026-07-22 → design.** Outcome: **NEX is not adopted as a live
substrate; soulrealm builds a from-scratch, NEX-influenced runtime** whose
single control plane is the soulstream topic op-log. `agent`/`tool` is kept as
a role axis, orthogonal to lifecycle. See episode 0002 and design doc
`hq/02-DESIGN/0001-soulrealm-runtime.md`.

Per bar:

- **Bar 1 (NEX carries the identity model) — PARTIAL, and moot under the
  chosen direction.** Issuance PASSES: the node mints a fresh uniquely-keyed
  scoped NATS user per workload (`internal/credentials/signing_key.go`),
  delivered via required `workload_creds` in an xkey-encrypted env `[measured]`.
  But the stock scope forbids publishing realm subjects (`WorkloadClaims` →
  Pub `_INBOX.>` only) and dev-mode issues nothing `[measured]`. The gap is
  closable through the public `WithMinter(models.CredVendor)` option without
  touching `internal/` `[measured]` — so *embedding* NEX was viable. We are not
  taking that path (see Bar 4), but the **design is adopted as influence**: a
  soulrealm-owned minter, scoping each workload's NATS user to its persona's
  soulstream subjects, plus the xkey-encrypted-env delivery.

- **Bar 2 (backend orthogonality) — not exercised; carried forward as a design
  requirement.** Only the native runtime ran. NEX proves the seam is
  expressible (`WithAgent`/nexlet SDK) `[measured]`; soulrealm's own backend
  abstraction (native/Docker/Firecracker/K8s) inherits constitution III as a
  `[D]` design obligation, validated when the first two backends land.

- **Bar 3 (role orthogonal to lifecycle) — PASS `[measured]`.** NEX exposes two
  native axes, `--type` (runtime) and `--lifecycle` (service | function | job);
  neither is a role axis. A `tool` legitimately spans lifecycles (persistent
  MCP server vs on-demand exec), so role is not a lifecycle synonym. `agent`/
  `tool` is a real, independent axis and is soulrealm's to define.

- **Bar 4 (lowest layer that works) — reframed by the direction call.** The
  bar assumed we would build *on* NEX and asked for the least-invasive layer.
  The measured layers were all reachable (embed via public options, no fork
  needed) `[measured]`. But the decision rejects building on NEX at all, for a
  reason outside the bar's frame: **two control planes.** NEX runs its own
  (`$NEX.control.*`, auctions, `$NEX.agent.*`); soulrealm's constitution I puts
  the topic op-log as the single control plane. Eliminating the second plane —
  and fitting the runtime to soulrealm's specific needs — is judged
  `[judgment]` to outweigh the reuse embedding would buy. Naming bonus: not
  depending on NEX frees the word "agent" from NEX's node-runtime collision, so
  the role keeps its natural name.

**Decision class:** `[judgment]` built on `[measured]` findings. The measured
work closed the *feasibility* questions (embedding works, role is orthogonal,
issuance is free); the fork/embed/rebuild *choice* among viable options is a
judgment, closed by teach-back (the maintainer restated the single-control-
plane argument) after the opposing case (embed-on-top) was argued at full
strength.

**Reversal condition (for the rebuild decision):** revert to embedding NEX
behind a single-control-plane adapter if, while building the runtime, we find
ourselves **reimplementing NEX's execution layer wholesale** rather than
borrowing its shape — concretely, if the isolation-backend + process-
supervision + artifact-fetch + scoped-minter machinery reaches rough parity
with NEX's nexlet layer in scope and we are maintaining it ourselves for no
capability NEX lacked. That reading says the second control plane was the
cheaper cost, and the trade should flip.

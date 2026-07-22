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

## Pre-registered bars

- **Bar 1 — NEX carries the identity model.** A spike launches two workloads
  on a real NEX node; each receives scoped NATS credentials and can publish a
  soulstream op attributed to a distinct persona *without* soulrealm minting
  or brokering credentials itself. PASS if the substrate delivers per-workload
  scoped identity end-to-end; FAIL if soulrealm must run its own credential
  tier to get per-persona attribution (which would strain constitution II).

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

<Empty until graduation.>

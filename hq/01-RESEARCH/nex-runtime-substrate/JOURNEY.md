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
pre-registered to test substrate fit and the correct layer. No spike run yet —
next step is Bar 1 (scoped-identity end-to-end on a real NEX node).

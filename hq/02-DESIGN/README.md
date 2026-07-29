# 02-DESIGN — Soulrealm system specification (document set)

This set specifies, without ambiguity and from a functional point of view, the
system to be built. It defines **what must exist** and **how each part
behaves**, not the reasoning behind the choices. An implementer should be able
to build a working system from these documents without needing undocumented
decisions.

**The spec-kit rule:** every document here is written explicit enough to be the
argument to `/speckit-specify` — the capability, its seams, its configuration
surface, and its acceptance criteria, with no guessing left to the spec
writer. New documents take the next free `NNNN-` number (`0001-…` onward).
Graduating research enters through `/research-graduate`; behavioral changes
made during implementation propagate back here (see
[`../00-GENESIS/how-we-work.md`](../00-GENESIS/how-we-work.md)).

## Documents

| # | Document | Covers | Status |
|---|---|---|---|
| 0001 | [`0001-soulrealm-runtime.md`](0001-soulrealm-runtime.md) | The runtime architecture: single control plane on the op-log, the role×lifecycle workload model, per-workload identity, lifecycle-as-ops, pluggable backends, the NEX influence ledger | graduated from `nex-runtime-substrate` (episode 0002) |
| 0002 | [`0002-kubernetes-backend.md`](0002-kubernetes-backend.md) | The Kubernetes isolation backend: pod-per-workload behind the unchanged seam — generic-image artifact delivery, Secret credential delivery, watch-based supervision, backend-not-scheduler scope guard | graduated from `kubernetes-backend` (episode 0008) |

Read 0001 first — it is the map. It fixes the decided shape and marks the `[O]`
sub-questions (minter signing story, multi-node placement, backend details)
that need their own design/spec pass before they are built.

## Status legend (used once documents exist)

Every component and requirement will carry one of these tags. They describe
**validation maturity**, not importance.

- **[V] Validated** — confirmed by a spike against a real NEX node / backend.
  Build as specified.
- **[D] Design** — fully specified functionally, but not yet validated. Build
  as specified; expect refinement once it runs.
- **[O] Open** — the interface and a default behavior are specified, but the
  best internal is a known unsolved problem. Build the interface and the
  default; expect the internal to be replaced. **[O]** items are where
  implementation risk concentrates.

## Requirement language

- **MUST** / **MUST NOT** — mandatory / prohibited.
- **MAY** — permitted, not required.
- A value given as a *default* is the value shipped unless configuration
  overrides it.

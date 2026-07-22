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

## Status

**Empty by design.** No design has graduated yet. The runtime is deliberately
specified last, against decided research (constitution IV). The first document
lands when [`01-RESEARCH/nex-runtime-substrate`](../01-RESEARCH/nex-runtime-substrate/README.md)
graduates — expected to define the substrate choice, the workload contract(s),
and the backend seam.

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

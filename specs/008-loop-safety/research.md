# Research: Loop safety — the wake budget

No open unknowns: the mechanism was decided by measurement in soul-hq
research episode 0128 (the graduated topic `loop-safety`; design
[`0006-loop-safety.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0006-loop-safety.md)).
This file records the decisions this build inherits, with their rationale
and the alternatives that were measured out.

## R1 — The budget is composed: window floor + depth bound

- **Decision**: enforce both an authorship-window floor (K own turn.posts
  per topic per W) and a provable-chain depth bound (D hops over the
  `WakeOpID` binding); refuse when either crosses.
- **Rationale**: each alone fails a measured case. Depth alone is evaded
  by self-posted outcomes (arbitrary op ids carry no binding: 393 turns
  in 3s past a D=4 gate, 0 refusals). The window alone is coarser than
  depth on provable chains and blind to chain shape. Composed, every
  cascade the rig produced halted at its pre-computed bound.
- **Alternatives considered**: depth-only (measured out, above);
  window-only (halting holds but loses the tight per-chain bound and the
  diagnostic); "agents never wake agents" (fails legitimate delegation —
  Bar 3 exists precisely to discriminate this); rate limiting in wrapper
  process memory (violates record-is-position: state beside the log,
  lost on restart).

## R2 — The gate sits at admission, refusals op-less

- **Decision**: evaluate after self-skip and outcome-existence pre-check,
  before the harness; a refusal posts nothing and logs one structured
  line.
- **Rationale**: measured. A refusal expressed through the harness slot
  becomes a self-report tapping the asker — the wrapper's own outcome
  contract re-arms the loop (312 agent turns, 156 self-reports in 4s).
  Op-less refusal leaves the mention in the inbox window and the
  deterministic outcome id keeps at-most-one outcome — exhaustion is a
  delay, not a loss.
- **Alternatives considered**: refuse via HarnessResult (measured out,
  above); post a refusal op (an op is a wake source — 0083's shape);
  ack-and-drop the mention (a loss, violating the record-is-position
  catch-up contract).

## R3 — Everything computes from the view already read

- **Decision**: both budget parts and the walker take the `[]Turn` view
  `handleWake` already materialises; no second read, no new state.
- **Rationale**: the admission point already holds the view (it needs it
  for the outcome-existence check and the prompt anchor). The rig
  computed both budgets and full ancestry from exactly this view at
  cascade rates (84 wakes/s sustained with per-wake materialise).
- **Alternatives considered**: a JetStream consumer position or KV
  counter (infrastructure beside the record — constitution S1/I); a
  notify-payload hop counter (a core wire change design 0006 explicitly
  avoids; also forgeable by any client that crafts notifies).

## R4 — The view needs Timestamp and Mentions? Timestamp only

- **Decision**: grow wrap's local `Turn` type by the contribution
  timestamp (the window clock). Mentions are not needed by the gate or
  the walker (the binding is the outcome id, not the mention text).
- **Rationale**: the window floor counts the persona's own turn.posts in
  W — that needs author, type, timestamp. The depth walk needs op ids
  and authors only. The rig's walker never consulted mentions for
  resolution (421/421 resolved by id binding alone).
- **Alternatives considered**: carrying the full core Contribution into
  wrap (wider coupling than the correlate half needs — the narrow local
  type is the file's stated pattern).

## R5 — Defaults on: D=4, K=8, W=10m

- **Decision**: defaults as design 0006 §3; either knob zero disables
  that part; both zero = no gate, one startup line names the unbudgeted
  standing.
- **Rationale [judgment, carried from the design]**: generous against
  every legitimate flow measured (delegation depth 3, single answers),
  orders of magnitude under the danger numbers (84 wakes/s cycle,
  1,264.7 ops/s colony). Default-on because the danger is measured and
  the opt-out is one explicit config away.
- **Alternatives considered**: default-off (silently leaves the measured
  runaway live in every deployment — rejected by the design); higher
  defaults (nothing measured asks for them; tighten later by demand).

## R6 — Ambiguity in the walk is reported, never absorbed

- **Decision**: `ParentOf` returns the match count; the chain walk
  surfaces ambiguity to its caller.
- **Rationale**: UUIDv5 collision is practically impossible (0 in 421
  resolved chains) but a silent wrong pick would corrupt both the gate
  and the diagnostic; reporting costs nothing.
- **Alternatives considered**: first-match-wins (silent); erroring the
  wake (an availability hole hung on an impossibility — logging and
  refusing conservatively is not needed either; the count simply rides
  the return).

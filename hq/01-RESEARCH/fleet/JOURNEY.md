# fleet — investigation journey (started 2026-07-29)

## 2026-07-29 — desk findings before any spike; Bar 2 amended openly

Reading soulstream's normative docs against the fleet question (no code run
yet) produced three findings and one amendment:

- **Sub-topics don't rescue transient signaling** `[mechanism-argument]`.
  The proposal was to put fleet coordination on a sub-topic of the current
  topic. Sub-topics live at `SOULSTREAM.TOPICS.OPS.<parent>.<child>`
  (core/03-topics.md), which is still inside the `SOULSTREAM` stream's
  `SOULSTREAM.>` capture (core/01-protocol.md). The M1.2 lesson was about
  stream capture semantics (persist + ack racing request-reply), not
  namespace collision — so bids/heartbeats on a sub-topic would still be
  persisted chatter, and would also break the protocol's per-topic growth
  bound (baseline + op tail). Transient traffic stays off-stream
  (`SOULREALM.SVC.*`) or must not exist.
- **Soulstream already prescribes claim-race** `[measured against the
  spec text]`. work.md stage 2: any persona may publish `work.claim`;
  when two race, *first claim in stream order wins, later claims are void
  by projection — no lock service, no arbiter*. Bar 1's
  claim-race-first protocol is therefore the house rule, not a novel
  candidate; the spike's job is to measure whether it holds at fleet
  racing rates.
- **Liveness may need no heartbeats at all** `[mechanism-argument]`.
  work.md also gives a *deterministic idle rule*: a timed-out claim
  reopens the item **by projection**, with no emitter. Fleet liveness
  could be absence-of-durable-progress rather than presence-of-pings —
  which would dissolve most of the second-control-plane risk. Where
  sub-topics *do* earn their keep: a chatty long execution can stream
  progress ops on `<topic>.<exec>` to keep the parent readable, while
  `SOULSTREAM.TOPICS.OPS.<topic>.>` still delivers the tree (03-topics.md's
  "keep focus without fragmenting").

**Bar 2 amendment (before any run).** As registered, Bar 2 presumed the
mechanism: "closes as `work.abandon` on the stream, *emitted by a surviving
party*". work.md's timeout-by-projection is a competing mechanism with no
emitter, and pre-committing to the wrong one would make the bar degenerate.
Amended to: the item must verifiably leave the claimed state within the
bound, by explicit abandon **or** by projection rule, mechanism recorded.
Thresholds (10/10 kills, ≤ 2× liveness interval, zero double-closes, no
resurrection without declared re-placement) are unchanged. No experiment had
run at amendment time.

## 2026-07-29 — spike 1: claim-race placement — Bar 1 criteria all met

**Protocol** (spike code in the session scratchpad, throwaway; recorded here
for re-creation). Hermetic in-process NATS/JetStream (nats-server 2.14.3, the
`internal/natstest` pattern). An `opener` persona provisions the realm and
starts topic `assembly`. Two **separate node OS processes** (`node-a`,
`node-b`), each its own connection and persona, run the same loop:
`topic.Follow` the op-log; on first sight of an item in `open` state, publish
`work.claim` immediately; when the folded view says `Owner == me`, record the
launch as a comment anchored to the item (`launched-by:<persona>`). No other
traffic of any kind. The harness opens 20 items sequentially, waits for
launch evidence per item, then a **third, fresh observer** materialises from
the stream alone for the verdict. Six runs total (1 + `-count=5`).

**Results** `[measured]`:

- **Exactly-one-launch: 120/120 rounds** across six runs (bar needed 20/20).
  Zero double-launches, zero never-launched.
- **Every round genuinely contested**: 120/120 rounds saw both nodes claim
  (mean 2.00 claims/round) — these were races, not walkovers. Win splits per
  run: 11/9, 14/6, 10/10, 12/8, 10/10, 10/10 — no starvation.
- **Replay alone reconstructs placement**: the fresh observer's fold named
  the placing node every round; first non-void claim == folded owner ==
  launch-evidence author; every losing claim folded void with a strictly
  later stream sequence.
- **Zero non-record subjects on the stream**: per run, the whole realm was
  `SOULSTREAM.TOPICS.INFO.<topic>` = 1 and `SOULSTREAM.TOPICS.OPS.<topic>`
  = 81 (1 start + 20 open + 40 claim + 20 evidence). No `SVC` traffic, no
  auction, no bids — claim-race needed **no transient signaling at all**.

**Caveats** `[mechanism-argument]`: the "launch" is stubbed as an anchored
evidence op — the backend seam is proven M1.1–M2.1 machinery and was not the
variable under test. Single-server JetStream: stream order is one server's
serialization; a clustered stream keeps a single total order through raft,
but that is unmeasured here. Both node processes shared one host; the
property leaned on is total order, not timing, so network asymmetry should
not change the fold — untested.

**Conclusion**: Bar 1's criterion is met by soulstream's existing house rule
with zero new machinery. Placement *is* `work.claim`. No auction variant was
needed (per protocol, it is only attempted if claim-race fails).

## 2026-07-29 — pre-registering spike 2 (Bar 2) parameters, before any run

Per the amended Bar 2, naming the liveness interval **before** the runs:
**sweep window 2 s, sweep cadence 500 ms** (spike scale), so the bound is
**≤ 4 s from SIGKILL** to the item leaving `claimed`. Mechanism under test:
the hybrid the library already implies — `upkeep.StaleClaims` (pure
projection detector: claimed items whose newest anchored activity is older
than the window) + an ordinary `work.abandon` published by **any surviving
sweeper**, with racing sweeps converging because the second abandon folds
void. Both nodes sweep. 10 kill rounds (SIGKILL the owner mid-"workload",
survivor must reopen the item within the bound; the harness then closes the
item and spawns a replacement node). Plus two control rounds: a live owner
emitting progress comments at 1 s cadence for 3× the window must **not** be
abandoned; a live but **silent** owner is expected to be falsely abandoned at
~window — measuring that false positive is the point: it quantifies the
tension that absence-of-progress liveness makes progress cadence a *floor*
for long-running work (work.md stage 4 already says runners "stream progress
as ops", but the growth bound and the sub-topic question then apply).

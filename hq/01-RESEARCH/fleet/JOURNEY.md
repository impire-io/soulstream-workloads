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

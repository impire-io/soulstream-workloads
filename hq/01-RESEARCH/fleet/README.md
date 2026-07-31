# Fleet — can the op-log alone place work across heterogeneous nodes and survive their deaths?

**State:** graduated
**Started:** 2026-07-29

## Abstract

Phase 2 closed with three backends behind one seam, all supervised by a single
runner on a single machine. Fleet is the horizon where more than one node
serves the same realm — heterogeneous (a Mac on microsandbox, a bare-metal
native node, a k8s cluster) and location-transparent. Episode 0002 settled
that fleet coordination must be ops on the soulstream log (no second control
plane) and explicitly deferred "multi-node placement-as-ops" as an open
sub-question; NEX answered the same need with `$NEX.control.*` auctions —
transient RPC, exactly the shape the constitution rejects — so its answer can
only be translated, not copied. A decisive answer here unlocks the Fleet
milestone(s) of Phase 3 and settles the ops/RPC boundary for scheduling; a
failure triggers episode 0002's reversal condition.

## The question

Can location-transparent placement across heterogeneous soulrealm nodes live
in the op-log as the **single** control plane — exactly-one node launches each
declared workload, a dead node's work still closes as `work.abandon`, and a
node without the realm signing seed still launches scoped workloads — without
reintroducing a coordinator service or polluting the stream of record with
transient chatter?

## Pre-registered bars

Written before any spike runs. The spike substrate for all three: two
soulrealm runner processes ("node A"/"node B", native backend suffices;
heterogeneity represented by distinct node tags) against a real
NATS/JetStream, reusing the existing hermetic/operator-mode test machinery.

- **Bar 1 — placement lands as record, not chatter.** With both nodes live
  and observing the same workload declaration, **exactly one** node launches
  it in **20/20 repeated launches** (zero double-launches, zero
  never-launched), the placement decision is reconstructible from the
  persisted stream alone (replay names the placing node with no access to
  either node's local state), and no transient signaling (bids, probes,
  heartbeats) rides `SOULSTREAM.>`. Protocol: try claim-race-on-the-log
  first (nodes race `work.claim`, M1.2-style transient traffic on
  `SOULREALM.SVC.*` only); an auction variant is attempted only if
  claim-race measurably fails, with the raw failure numbers recorded in
  JOURNEY.md.
- **Bar 2 — a dead node cannot leak open work.** SIGKILL the placing node
  mid-workload (no graceful-shutdown path runs): in **10/10 kills** the work
  item verifiably leaves the claimed state within **≤ 2× the spike's chosen
  liveness interval** (interval named in JOURNEY.md before the runs) — by
  either mechanism: an explicit `work.abandon` from a surviving party lands
  on the stream, **or** the claim expires by deterministic projection rule
  (work.md's timed-out-claim reopen), such that stream replay alone shows
  the item reopened or closed. The operative mechanism is recorded in
  JOURNEY.md; zero double-closes; no resurrected copy unless re-placement
  is explicitly declared. *(Amended openly 2026-07-29, before any run —
  see JOURNEY.md.)*
- **Bar 3 — scoped launch without the seed.** Node B launches a workload
  whose credential is minted without the realm signing seed ever existing on
  node B's disk (asserted in-test, as the k8s Secret path did): the
  workload's in-guest scope probe passes in-scope and is **denied
  out-of-scope** against an operator-mode server (the SC-003 probe, reused),
  and the mint path's revocation/expiry story is named in the topic's
  conclusions. Protocol: minting delegated over the realm (transient,
  `SOULREALM.SVC.*`) or a scoped delegated key — whichever the spike
  discriminates as viable.

## Reversal condition

If making Bars 1–2 pass requires machinery at rough parity with NEX's
`$NEX.control.*` layer — a standing coordinator/scheduler service, or
per-heartbeat traffic that must be persisted to be correct (i.e., the record
is only trustworthy if it also carries the chatter) — then "placement-as-ops"
is a second control plane in all but name, and episode 0002's own reversal
condition is in play: embed NEX behind a single-control-plane adapter rather
than rebuild its fleet layer from scratch.

## Verdict

**Answer: yes — measured on all three bars.** The op-log alone places work
and survives node death, with transient evidence allowed only to *delay*
decisions, never to make them. Graduated to design
`hq/02-DESIGN/0003-fleet.md` (episode 0010).

- **Bar 1 — PASS** `[measured]`. Exactly-one-launch in **120/120** rounds
  across six spike runs (bar needed 20/20), every round genuinely contested
  (both nodes claimed, mean 2.00 claims/round; win splits 14/6 → 10/10 — no
  starvation). A fresh third observer reconstructed the placing node for
  every round from stream replay alone (first non-void claim = folded owner
  = launch evidence). The whole realm was two topic subjects — zero `SVC`
  traffic, zero auction ops: placement **is** `work.claim`, soulstream's
  existing house rule. The auction variant was never needed.
- **Bar 2 (as amended openly before any run — see JOURNEY) — PASS**
  `[measured]`, both variants, 10/10 kills each within the ≤ 4 s bound
  (window 2 s, cadence 500 ms, named before the runs): baseline min/mean/max
  2.02/2.37/2.53 s; probe variant 2.01/2.37/2.55 s. Zero double-closes
  (racing sweeps converge — the second abandon folds void), zero
  resurrections, no abandon authored by a dead owner. Mechanism recorded:
  **projection nominates (`StaleClaims`), transient evidence vetoes
  (probe-before-abandon on core NATS, outside the stream capture), the log
  decides (ordinary `work.abandon`)**. Controls: a progress@1 s owner was
  never abandoned; a live-silent owner was falsely abandoned at 2.5 s in
  the baseline and **never** with the probe (8 vetoes) — and the probe is
  free on true deaths (a dead node's subscription dies with its connection
  → instant no-responders) `[measured]`.
- **Bar 3 — PASS** `[measured]`. A launching node with no signing material
  — seed asserted absent from its scratch tree, argv, env, and output —
  obtained a scoped credential over transient `SOULREALM.SVC.*`
  request-reply and ran the byte-identical SC-003 probe against an
  operator-mode server: in-scope allowed, out-of-scope denied; negative
  control confirmed enforcement. Expiry floor measured: a 2 s-TTL
  credential disconnected **10 ms** after expiry. Revocation story named:
  TTL/exp as the floor `[measured]`; account revocation list or signing-key
  rotation as escalation `[mechanism-argument]`.

**Post-bar follow-up, measured:** spike 4 reversed spike 3's judgment
against scoped delegated keys — one scoped signing key with a tag-template
(`{{tag(topic)}}`) clamps permission-less users per-workload, server-side,
in both directions, and rejects a self-permissioned user outright. The
identity plane (`../soulidentity`) is the preferred minting/enrollment
home; spike 3's delegated minting stands as the measured fallback. The
reversal condition never fired: no coordinator, no auction, no persisted
chatter anywhere.

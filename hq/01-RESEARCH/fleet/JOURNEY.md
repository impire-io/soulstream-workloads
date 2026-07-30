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

**Probe variant, registered before any run (additive).** A second sweeper
variant answers the false positive with transient evidence instead of
durable chatter: before abandoning a stale candidate, the sweeper sends one
core-NATS request-reply probe to the owning node —
`SOULREALM.NODE.<realm>.<persona>.PING`, **outside** the `SOULSTREAM.>`
JetStream capture, probe timeout **500 ms** (inside the 4 s bound). The
governing rule: **evidence, not authority** — a reply only vetoes *this
sweep's* abandon (delays a decision); silence lets the abandon proceed;
every state transition remains an ordinary op; replay stays complete
without the probes. Failure degrades toward reclaim, never toward leak.
Sweepers skip items they themselves own in both variants. Both variants
run the full protocol: **10 kill rounds each** against the same ≤ 4 s
bound, plus the two control rounds — progress\@1 s must be abandoned in
neither variant; the live-silent owner is expected falsely abandoned at
~window in the baseline (measured) and **not at all** over 3× window in
the probe variant. Known limits registered up front: a probe cannot
distinguish dead from partitioned (false abandon → deterministic fold
collision → at-least-once semantics, to be named in the design), and a
zombie that answers probes without progressing can suppress reclaim —
cap policy is design-doc material, not spiked here.

## 2026-07-29 — spike 2: node death — Bar 2 met in both variants

**Protocol as pre-registered** (spike code in the session scratchpad; same
bench as spike 1, node binary extended with the sweeper, the PING responder,
and body-directed progress mode). Per variant: two control rounds, then 10
kill rounds — open item, wait for claim + launch evidence, SIGKILL the
owner's process (no graceful path), measure from the kill to the fold
showing the item out of `claimed`; harness then closes the item and a
replacement node joins so every round stays two-node.

**Baseline (registered mechanism: `StaleClaims` nominates, a surviving
sweeper's ordinary `work.abandon` decides)** `[measured]`:

- **10/10 kills reopened within the ≤ 4 s bound**: min 2.02 s, mean
  2.37 s, max 2.53 s from SIGKILL.
- **Zero double-closes** (exactly one non-void abandon per kill — racing
  sweeps converged as designed), **zero resurrections** (no non-void claim
  after any abandon), and no abandon ever authored by the dead owner.
- Control progress\@1 s: never abandoned across 3× window — anchored
  progress keeps the claim fresh.
- Control live-silent: **falsely abandoned at 2.5 s** — the pre-registered
  false positive, now quantified: absence-of-progress liveness makes the
  sweep window a hard progress-cadence floor.

**Probe variant (probe-before-abandon)** `[measured]`:

- **10/10 kills within the bound at statistically identical cost**: min
  2.01 s, mean 2.37 s, max 2.55 s — the probe added no measurable latency
  to true positives.
- Why it is free on true positives: a dead node's PING subscription dies
  with its connection, so the probe fails **instantly** with NATS
  no-responders instead of consuming the 500 ms timeout; the full timeout
  is only paid when the owner is subscribed but unreachable — a true
  partition `[measured via the latency comparison; mechanism-argument for
  the explanation]`.
- Control live-silent: **zero false abandons** across 3× window; 8 probe
  vetoes visible in node stderr logs — deliberately log-level evidence,
  not stream record.
- Probes rode `SOULREALM.NODE.<realm>.<persona>.PING` (core NATS); the
  `SOULSTREAM` stream captures only `SOULSTREAM.>`, so probe traffic
  cannot pollute the record by construction `[mechanism-argument]`.

**Mechanism recorded** (as the amended bar requires): the hybrid —
**projection nominates** (`StaleClaims`), **transient evidence vetoes**
(probe reply; evidence-not-authority), **the log decides** (an ordinary
`work.abandon`; the second abandon folds void, observed as zero
double-closes across 21 abandons in 2×10 kills + 1 baseline false
positive).

**Caveats** `[mechanism-argument]`: single host, shared clock —
`StaleClaims` compares author-claimed op timestamps against the sweeper's
local clock, so cross-node skew widens or narrows the effective window in a
real fleet (the design must state a tolerance). Partition-vs-death stays
undecidable; the semantic is at-least-once with the fold resolving the
collision deterministically. The zombie-suppression cap remains a design
question, not spiked.

**Where the bars stand**: Bar 1 measured PASS-shaped (spike 1), Bar 2
measured PASS-shaped in both variants (this spike). Remaining before
graduation: **Bar 3** — scoped launch without the signing seed on the
launching node.

## 2026-07-30 — spike 3: delegated minting without the seed — Bar 3 met

**Protocol** (spike code in the session scratchpad, throwaway). Operator-mode
server that enforces user JWT permissions (the SC-003 machinery,
`internal/natstest/operator.go` pattern). The harness stands in for the
**enrollment authority**: it issues each node one ordinary scoped node
credential — never signing material. **Node A** (minter node, separate OS
process) holds the account signing seed via env (vault stand-in) and serves
delegated minting on `SOULREALM.SVC.MINT.<realm>` — transient request-reply,
using the repo's real `minter` package unchanged. **Node B** (separate OS
process) holds no signing material: it requests one workload credential,
writes the minted user credential into its own scratch dir (the k8s Secret
analog), and launches the repo's **real `cmd/scope-probe` binary** with the
M1.1 env contract.

**Results** `[measured]`:

- **Negative control**: the operator server refuses an unauthenticated
  connection (Authorization Violation) — so the denials below prove
  enforcement, not misconfiguration.
- **Scope holds across the node boundary**: probe exit 0 — in-scope publish
  allowed, out-of-scope publish denied by the server, with the byte-identical
  SC-003/SC-004 probe.
- **The seed never reaches node B**: asserted absent from node B's entire
  scratch tree (only the minted creds file exists there — user JWT + user
  seed), from its argv, its environment, and its captured output.
- **Expiry floor measured**: a 2 s-TTL minted credential was actively
  disconnected by the server **10 ms after its expiry** ("authentication
  expired") — expiry is server-enforced on live connections, not advisory.

**Findings**:

- **Node enrollment is a real fleet requirement** `[measured]`: operator mode
  rejects anonymous connections, so a node must be enrolled before it can
  even ask for a mint. Fleet needs a once-per-node enrollment act issuing an
  ordinary scoped node credential; the authority is wherever signing
  material lives (episode 0002's named impire-tenants/vault tie). Minimal
  permission sets discovered: minter node `{pub _INBOX.>, sub MINT}`,
  launching node `{pub MINT, sub _INBOX.>}`.
- **Revocation story, named** (the bar requires it): the floor is JWT `exp`,
  server-enforced (measured at 10 ms); the escalation is the account
  revocation list pushed through the resolver, or rotating the signing key
  to invalidate everything it signed `[mechanism-argument]`.
- **Alternative discriminated** `[judgment]`: a scoped *delegated signing
  key* on each node would multiply seed-grade secrets and its static
  permission template cannot express per-persona+topic dynamic scopes
  cleanly; delegated minting reuses the existing minter seam unchanged and
  keeps one seed holder. Revisit if minter availability becomes the launch
  bottleneck — the minter node is a single point of failure for *launches*
  (never for running workloads), mitigable with a queue group of minter
  nodes.
- Mint traffic rides `SOULREALM.SVC.*` transient request-reply — consistent
  with the M1.2 rule; the signing seed never crosses the wire (only the
  fresh per-workload user JWT + user seed do).

**Caveats** `[mechanism-argument]`: the harness stood in for the enrollment
authority — the real enrollment ceremony and the seed's home (vault) are
design material. Single realm account; multi-account fleets unexplored.

**Where the bars stand**: all three pre-registered bars measured PASS-shaped
(spike 1: placement; spike 2: node death, both variants; spike 3: seedless
scoped launch). The topic is ready for `/research-graduate fleet --to
design`.

# Episode 0010 — Fleet: the log nominates, evidence vetoes, the log decides (2026-07-29 → 2026-07-31)

With three walls standing (native, microVM, pod), the maintainer asked
whether Fleet — more than one node, location-transparent scheduling — was
the next gate. Episode 0002 had deferred exactly this ("multi-node
placement-as-ops"), and NEX's answer (`$NEX.control.*` auctions) was the
shape the constitution forbids. The `fleet` research topic pre-registered
three bars before any spike ran; four spikes later **all three are PASS
[measured]**, plus one post-bar follow-up that reversed a judgment made two
days earlier:

- **Placement is `work.claim`.** Two node processes racing soulstream's
  house rule (first claim in stream order wins, losers void by projection)
  produced exactly-one-launch in **120/120 rounds** across six runs, every
  round genuinely contested, with placement reconstructible by a fresh
  observer from stream replay alone. The whole realm was two topic
  subjects: **no auction, no bids, no transient signaling at all**
  [measured]. The pre-registered auction fallback was never attempted —
  claim-race never failed.
- **A dead node cannot leak open work, and liveness needs no heartbeats.**
  SIGKILLing the owning node mid-workload, 10/10 kills per variant closed
  within the ≤ 4 s bound (mean 2.37 s both variants): `upkeep.StaleClaims`
  **nominates** by pure projection, a surviving sweeper's ordinary
  `work.abandon` **decides**, and racing sweeps converge (the second
  abandon folds void — zero double-closes observed) [measured]. The
  registered baseline falsely abandons a live-but-silent owner at ~window
  (measured at 2.5 s); the **probe-before-abandon** variant — one core-NATS
  request-reply ping, *outside* the stream capture, governed by
  **evidence-not-authority** (a reply only delays; every transition stays
  an op) — eliminated the false positive entirely at statistically
  identical reclaim cost, because a dead node's subscription dies with its
  connection: the probe fails instantly with no-responders, so the timeout
  is only ever paid in a true partition [measured].
- **The seed never travels.** A launching node holding no signing material
  (asserted absent from disk, argv, env, output) obtained a scoped
  credential over transient `SOULREALM.SVC.*` request-reply and ran the
  byte-identical SC-003 probe against an operator-mode server: in-scope
  allowed, out-of-scope denied. The expiry floor is real and fast — a
  2 s-TTL credential was disconnected **10 ms** after expiry [measured].
  Discovered along the way: operator mode rejects anonymous connections,
  so **node enrollment is a first-class fleet requirement** — a node needs
  an ordinary scoped node credential before it can even request a mint.

**Reversed, twice — openly.** Bar 2 was amended *before any run* when
soulstream's own work.md revealed timeout-by-projection as a competing
mechanism to "a surviving party emits abandon" (the measured answer is the
hybrid of both). And spike 3's judgment against scoped delegated signing
keys fell to spike 4's measurement: **one** scoped signing key whose
template derives subjects from user tags (`{{tag(topic)}}`) clamps
permission-less user JWTs per-workload, server-side, in both directions —
and rejects a self-permissioned user at connection time, a security
property the JWT-embedded model lacks [measured]. With `../soulidentity`
(shipped M1/M3/M4; vault custody, xkey-sealed mint surface, auth-callout
enrollment) as key custodian, both original objections dissolve. The
minter role therefore **dissolves into the identity plane**: no soulrealm
node is a minter, every fleet node is homogeneous.

What it taught: the M1.2 boundary generalizes into the fleet control-plane
rule — **the log nominates, transient evidence vetoes, the log decides** —
and sub-topics, examined for heartbeats, cannot rescue transient signaling
(they live inside the `SOULSTREAM.>` capture) but do earn their keep for
chatty per-execution progress [mechanism-argument]. Named honestly:
partition-vs-death is undecidable, so fleet semantics are **at-least-once**
with the fold resolving collisions deterministically; absence-of-progress
liveness makes the sweep window a progress-cadence floor for long-running
work; a zombie that answers probes without progressing needs a cap policy;
single-host clocks and single-server JetStream leave clock-skew tolerance
and clustered total order unmeasured.

What it opened: design [`0003-fleet.md`](../02-DESIGN/0003-fleet.md) — the
homogeneous fleet node (claim-race placement, sweep + probe reclaim,
identity-plane enrollment and minting, tag-template scopes preferred with
spike-3 delegated minting as measured fallback), carrying the open
internals honestly. Roadmap Phase 3 (Fleet) is unblocked.

Reversal condition: if fleet-scale racing shows claim-race degrading
(placement latency or void-claim churn growing with node count) so that a
standing coordinator or auction becomes *required* rather than optional,
episode 0002's own reversal is in play — embed NEX behind a
single-control-plane adapter rather than rebuild its fleet layer. If
tag-template scopes prove unable to express a required workload scope, the
minting story reverts to spike 3's measured delegated-minting fallback.

Trail: research topic `hq/01-RESEARCH/fleet/` (removed at graduation; full
history in git) — opened `a2146b7`, Bar 2 amendment `25fdb56`, spike 1
`0580a44`, probe pre-registration `2e1d15a`, spike 2 `b79cdc5`, spike 3
`d506ec8`, identity-plane desk findings `967f6ce`, spike 4 `e8ff821`,
verdict `04a3aca`;
design [`0003-fleet.md`](../02-DESIGN/0003-fleet.md); spike code in the
session scratchpad (throwaway, per how-we-work).

# 0003 — Fleet

**Status of this document:** graduated from the `fleet` research topic
(episode [0010](../04-JOURNEY/0010-fleet.md)); **not yet built** — the
spec-kit pass for the first Fleet milestone starts from here. All three
pre-registered research bars were **measured PASS** via spikes (placement,
node death, seedless scoped launch), plus a measured post-bar follow-up
(tag-template scoped minting). Tags mark what is validated **[V]** (by
research spike — no fleet feature has landed), decided **[D]**, and open
**[O]**, per [`README.md`](README.md).

Seam vocabulary per [`0001-soulrealm-runtime.md`](0001-soulrealm-runtime.md);
work vocabulary per soulstream's `work.md` extension.

---

## 1. The capability

A **fleet** is more than one soulrealm node serving the same realm:
heterogeneous machines (a laptop on the msb backend, a bare-metal native
node, a cluster behind the k8s backend), each running the same runner, all
observing the same topics — with **location-transparent placement** and
**no control plane other than the op-log** (constitution I/V).

The governing rule, generalized from the M1.2 boundary and measured
end-to-end in the research: **the log nominates, transient evidence
vetoes, the log decides.** Durable state transitions are ordinary ops on
the stream; transient traffic (probes, RPC) rides soulrealm's own
non-captured subjects and may only *delay* a decision, never make one.
Replay of the stream alone MUST reconstruct every placement and every
reclaim `[V]`.

**Node homogeneity `[D]`:** there are no node roles. No soulrealm node is
a minter (§5 — minting authority lives in the identity plane); every node
is a launching node distinguished only by its node-side configuration
(backend selection per M1.3, capacity/tags later). A fleet of one node is
the degenerate case and MUST behave identically to today's single runner.

## 2. Placement

- Placement **is** soulstream's `work.claim` `[V]`: every node observing a
  topic sees an open execution work item, publishes an ordinary claim, and
  the fold decides — first claim in stream order wins, later claims are
  void by projection (work.md's house rule, no lock service, no arbiter).
  The winner launches through its configured backend; losers do nothing.
  Measured: exactly-one-launch 120/120 contested rounds; placement
  reconstructible from replay alone; zero transient signaling needed.
- There is **no auction and no bid vocabulary** `[D]`. The research
  pre-registered an auction fallback and never needed it; reintroducing
  one is the episode-0010 reversal condition, not a design option.
- A node MUST claim only work it can actually run (backend available,
  artifact resolvable); refinements such as capacity- or tag-based
  *self-selection* (a node simply not claiming) are permitted because they
  need no new vocabulary `[D]`. Cross-node placement *preference* beyond
  self-selection is out of scope until felt `[O]`.
- Losing claims stay visible as void events — that is record, not noise
  (a lost claim is history) `[D]`.

## 3. Liveness and reclaim

- **Nomination** `[V]`: each node runs a sweeper on a node-configured
  cadence applying soulstream's `upkeep.StaleClaims` — a pure projection
  rule flagging claimed items whose newest anchored activity is older than
  the window. Sweepers skip items they themselves own.
- **Veto — probe-before-abandon** `[V]`: before abandoning a stale
  candidate, the sweeper sends one core-NATS request-reply probe to the
  owning node (`SOULREALM.NODE.<realm>.<node>.PING` — transient, outside
  the `SOULSTREAM.>` capture). A reply vetoes *this sweep's* abandon;
  silence lets it proceed. **Evidence, not authority** `[D]`: the probe
  carries no state, appears nowhere in the record, and failure degrades
  toward reclaim, never toward leak. Measured: the probe eliminates the
  false-abandon of a live-silent owner at statistically zero cost on true
  deaths (a dead node's subscription dies with its connection — instant
  no-responders; the timeout is only paid in a true partition).
- **Decision** `[V]`: the reclaim is an ordinary `work.abandon` from the
  sweeping node; the item reopens for a fresh claim race. Racing sweeps
  converge — the second abandon folds void. Measured: 10/10 kills per
  variant within ≤ 2× the sweep window, zero double-closes, zero
  resurrections, no abandon authored by a dead owner.
- **Semantics — at-least-once, named openly** `[D]`: partition-vs-death is
  undecidable; a partitioned-but-alive owner may be reclaimed and
  relaunched. The fold resolves the collision deterministically (a late
  `work.done` on a re-claimed item folds by the state machine). Workloads
  whose side effects cannot tolerate at-least-once MUST NOT be declared
  into a fleet until an exactly-once story exists `[O]`.
- **Progress cadence floor** `[D]`: absence-of-progress liveness makes the
  sweep window a floor on durable progress for long-running work — a
  runner streaming progress ops (work.md stage 4) stays fresh; the probe
  covers the gaps. Chatty per-execution progress MAY ride a sub-topic
  (`<topic>.<exec>`) to keep the parent readable — sub-topics are for
  durable-but-chatty record, never for transient signaling (they live
  inside the stream capture) `[D]`.
- **Zombie cap** `[O]`: an owner that answers probes without ever
  progressing can suppress reclaim indefinitely. Interface: the sweeper
  MUST support a cap (after N consecutive probe-vetoed sweeps of the same
  item, the veto expires); default and semantics to be decided at spec
  time.

## 4. Node identity and enrollment

- **Enrollment is a first-class act** `[V]` (discovered, not designed:
  operator mode rejects anonymous connections). A node joins a realm by
  receiving one ordinary scoped **node credential** — never signing
  material. Research-measured minimum for the minting flow:
  `{pub SOULREALM.SVC.MINT.<realm>, sub _INBOX.>}`; the full node scope
  (topic ops for claims/abandons, its `PING` subject, backend needs) is
  finalized at spec time `[O]`.
- **The enrollment authority is the identity plane** `[D]`:
  `soulidentity` — vault-held keys, xkey-sealed NATS surface, and the
  auth-callout lane (a node arrives with a bootstrap token and receives a
  TTL-bounded credential; TTL is the revocation propagation bound). The
  soulrealm side treats enrollment as configuration (§6); it MUST NOT
  implement its own authority. Episode 0003's soulstream-only scope is
  amended by this document: the platform's identity service is now real,
  shipped, and consumed — the `minter.Minter` seam was built for exactly
  this substitution.

## 5. Per-workload minting

Two measured paths; the identity plane is preferred `[D]`:

- **Preferred — tag-template scoped mint via soulidentity** `[V]` at the
  mechanism level: the realm account carries **one scoped signing key per
  role** (e.g. `soulrealm-agent`) whose template is today's
  `minter.PermissionSet` with the dynamic parts as tag functions
  (`SOULSTREAM.TOPICS.OPS.{{tag(topic)}}`,
  `SOULSTREAM.PERSONA.NOTIFY.{{tag(persona)}}`). Workload users are minted
  **permission-less** (`SetScoped`), carrying only tags; the server clamps
  per-workload. Measured: the clamp holds bidirectionally; the real
  SC-003 probe passes; a self-permissioned user signed by the scoped key
  is rejected at connection — **a compromised mint path cannot
  over-scope**, a property the fallback lacks. For ephemeral mints the
  workload-side keypair is generated locally and only the *public* key
  crosses the wire — no seed travels in either direction. Missing piece
  `[O]`: soulidentity's mint does not stamp tags today (its M2 invites
  consumer-proven additions); until it does, this path cannot ship.
- **Fallback — delegated minting** `[V]`: spike 3's shape, the current
  `SigningKeyMinter` behind a transient `SOULREALM.SVC.MINT.<realm>`
  request-reply served by whatever holds the seed. Measured end-to-end:
  the launching node's disk/argv/env/output provably never see the seed;
  scope enforced; expiry floor 10 ms. Weaknesses recorded: the reply
  carries the minted user seed across the (NATS-authenticated) wire, and
  the mint service must be trusted not to over-scope.
- **Revocation story** `[D]`: the floor is the JWT `exp` — measured
  server-enforced within 10 ms on live connections; short `CredTTL`
  remains the per-workload default. Escalation: the account revocation
  list via the resolver, or signing-key rotation invalidating everything
  it signed `[O]` (operational procedure undefined).
- Sub-topic descendant scoping (`{{tag(topic)}}.>`) is untested and
  today's `PermissionSet` grants exact subjects only — parity kept; revisit
  with the sub-topic progress question `[O]`.

## 6. Node-side configuration surface

All node configuration; none of it may appear in a declaration
(constitution III). Sketch for the spec pass `[D]`:

- `SOULREALM_NODE_NAME` — the node's persona for enrollment, probes, and
  claim attribution.
- `SOULREALM_NODE_CREDS` — the enrollment credential (file path today; the
  auth-callout bootstrap token lane when consumed).
- `SOULREALM_SWEEP_WINDOW` / `SOULREALM_SWEEP_EVERY` /
  `SOULREALM_PROBE_TIMEOUT` — liveness parameters (research spike scale:
  2 s / 500 ms / 500 ms; production defaults decided at spec time, probe
  timeout MUST fit inside the reclaim bound).
- Backend selection stays `SOULREALM_BACKEND` (M1.3) — heterogeneity is
  per-node configuration, invisible to declarations.
- Mint endpoint configuration (identity-plane service location or the
  fallback mint subject) `[O]` — shape depends on the soulidentity tags
  addition.

## 7. Known limits carried openly

- **Clustered JetStream** `[O]`: every spike ran single-server; the fold
  leans only on the stream's total order, which a clustered stream
  preserves through raft, but this is unmeasured.
- **Clock skew** `[O]`: `StaleClaims` compares author-claimed op
  timestamps with the sweeper's clock; spikes shared one host. The spec
  MUST state a skew tolerance (window ≫ expected skew) or move staleness
  to broker-stamped time.
- **msb non-loopback NATS** `[O]`: episode 0007's named limitation — the
  microVM backend needs the `public` net profile to reach a non-loopback
  realm server; this becomes real the moment a second node exists.

## 8. Acceptance criteria

The spec-kit feature(s) out of this document MUST demonstrate, on at least
two real node processes against one realm:

1. The byte-identical M1.1/M1.2 declarations, submitted once, run on
   **exactly one** node per work item across repeated contested launches,
   with placement reconstructible from stream replay alone and zero
   non-record subjects on the stream.
2. SIGKILL of the owning node mid-workload ends the item as
   `work.abandon` from a surviving node within the configured bound —
   zero double-closes, no resurrected workload without a fresh claim on
   the record.
3. A live workload owner is never reclaimed while it answers probes or
   streams progress; the probe traffic appears nowhere in the stream.
4. A node holding no signing material launches a scoped workload whose
   credential is enforced by an operator-mode server (SC-003 probe:
   in-scope allowed, out-of-scope denied), with the seed asserted absent
   from that node's disk, argv, env, and output.
5. Runner, declaration, and backend seams are untouched by fleet-ness: a
   single-node fleet runs today's suites unchanged.

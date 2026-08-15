# Phase 0 research — the waker

The milestone's research gate is met: the `agent-participation` topic
graduated with all four pre-registered bars measured PASS (soul-hq episode
0082; design 0004). This file consolidates what that research settled and
decides the open items this plan must fix before build. Decisions are
numbered D1–D6.

## Settled by the graduated research (episode 0082) — consolidated

- **The wake path works end to end** `[measured]`: durable consumer →
  admission probe → headless harness with a generated MCP config → typed
  terminal event → one outcome op, acked after the outcome exists.
- **The reply obligation must be the waker's** `[measured]`: with the
  posting tool removed from the harness surface, the reply still landed;
  tool calls are enrichment, the envelope is the waker's.
- **Correlation must diff the run's own snapshots** `[measured]`: with
  several mentions in one topic, stream-order anchoring let an earlier
  wake's reply swallow later mentions' answers. Before/after comparison of
  the persona's turns is the correct primitive.
- **Failure can never speak in the agent's voice** `[measured]`: a revoked
  agent is refused in ~2ms and cannot post at all; failure testimony is the
  waker's own (design 0004 §7).
- **Templates are sufficient for harness diversity** `[measured]`: a
  structurally different event grammar ran through a byte-identical waker
  binary on a template-only change (dot-path terminal mapping).
- **Credential lanes** `[measured]`: per-wake token-lane connections bind
  revocation at the next wake; `mint.ephemeral` yields run-bounded
  credentials (TTL enforced server-side) and is operator-gated — the waker
  mints, the agent cannot.
- **Harness idioms live in templates** `[measured]`: claude-code needs
  `--verbose` with `stream-json`, stdin closed, `--strict-mcp-config`,
  tool allow/deny flags; codex needs nested `msg.type` mapping and its own
  flags. None of this may reach waker code.

## D1 — Harness runs are waker-supervised subprocesses, not backend launches

Day one, the waker executes a wake as a directly supervised subprocess
(fresh run dir, sanitized env, process-group kill), exactly as the research
rig did. It does **not** ride `backend.Start` and the declaration seam.

Argued against at full strength: running wakes through the backend seam
would inherit three proven walls (native/msb/k8s) and constitution-III
symmetry. Rejected for now on S2 grounds `[mechanism-argument]`: a wake
runs the *operator's installed harness* — a CLI with the operator's own
auth state (claude login, codex login) — which no per-run OCI image or
microVM carries today; forcing the seam day one would make the sandbox
backends unusable for exactly the harnesses the feature exists to wake,
while adding an image/auth story no acceptance scenario needs. The seam
integration is the named growth path: **when a wake harness exists as a
distributable artifact** (observable: a registration pointing at an
artifact rather than a host command), wake execution moves behind the
backend seam and `LifecycleFunction` becomes that declaration's lifecycle.

## D2 — Registrations are waker configuration; declaration vocabulary waits

A registration (persona, credential lane, template) lives in a waker
configuration file, versioned by the operator. The workload `Declaration`
does not grow a trigger block in this feature. Rationale `[judgment]`: the
declaration's consumers are the runner and the fleet's claim path; wiring
trigger vocabulary into it before fleet placement exists (M3.1 unbuilt)
would be speculative generality S2 prohibits — a seam with no second
occupant. Reversal observable: a second registration writer (the shell's
Agents module wanting to publish registrations, or the fleet needing to
claim wakes) reopens the declaration question with a concrete consumer.

## D3 — Outcome idempotency has two halves, because the windows disagree

The record stream's duplicate-tracking window is protocol-mandated at
exactly 2 minutes (`realm.MinDuplicateWindow` `[measured]`), while the
waker's redelivery window (`AckWait` ≥ run budget + margin) can legally
exceed it. So "outcome posts are idempotent across redeliveries" (spec
FR-005) takes both:

1. **Deterministic publish id**: every outcome op is published with
   `Nats-Msg-Id` derived from the wake (`soulstream-workloads-wake-<notify-op-id>`),
   so any redelivery inside the duplicate window dedupes server-side.
2. **Redelivery pre-check**: on `NumDelivered > 1`, before invoking a
   harness, the waker scans the topic's raw stream messages for that
   `Nats-Msg-Id` header; if the outcome already landed, it acks and stops.
   This closes the crash-after-post-before-ack window at any redelivery
   distance, using only headers the record already carries.

## D4 — Probe outcomes map to exactly two non-admitted classes

- **Refused** (authorization violation): the registration was revoked —
  nak with a long delay, no op, wait for re-grant. The agent's silence is
  the design (0004 §4).
- **Unreachable** (transport error, timeout): the realm, not the agent, is
  the problem — nak with a short delay. No failure turn either way: a
  waker that cannot reach the realm cannot post one, and a revoked agent
  must not cause one in the agent's name.

Only an **admitted** wake can end in a failure turn, and that turn is the
waker's own (spec FR-006).

## D5 — The waker's own voice is ordinary configuration

The waker takes its persona name and credential (its own, not any
agent's) from its configuration, and posts failure testimony through its
own connection under its own authorship — an ordinary operated persona,
no new identity machinery (constitution workloads-II readthrough: the
*waker* is plane machinery like the runner and minter; every *harness*
run still sees only its agent's scoped credential).

## D6 — Hermetic proofs use scripted harnesses; real harnesses are opt-in

The default gate proves the wake protocol against scripted harnesses
emitting the two measured grammars (claude-shaped flat `type:result`,
codex-shaped nested `msg.type:task_complete`) — the research's own
stand-in precedent, now serving as the regression guard for template
generality (SC-004). A real-harness proof (actual `claude -p` against a
local realm) is an opt-in target in the M1.3/M2.1 pattern, requiring the
operator's installed harness and auth.

## D7 — Core grows the idempotent turn post; the wake id is a UUIDv5

Core already treats the op id as the duplicate-detection identity —
`record.NewID` documents "doubles as the message's Nats-Msg-Id"
`[measured]` — and its internal publish path accepts a preset id
(`publishOpWith`, used by rollup), but no exported turn API reaches it.
D3 therefore names a **cross-repo prerequisite**: soulstream-core exports
`PostTurnIdempotent(ctx, body string, mentions []string, opID string)` —
the public arm of the library duty the extensions design already lists
("idempotent publish via `Nats-Msg-Id`, retry-with-same-id") — riding the
existing preset-id machinery, purely additive, tagged v0.8.3. The wake's
outcome id is a **UUIDv5 derived from the notify op-id**, staying in the
id shape every reader already handles; the beyond-window redelivery
pre-check (D3.2) becomes "materialise and look for that op-id among the
persona's contributions" — no raw header scanning.

## D8 — The waker introduces slog, and says so

The repo has no logging today — operator output is `fmt.Fprintln(stderr)`
in `main` `[measured]`. A standing daemon whose refused wakes deliberately
produce no ops (D4) needs an observable trace or refusals are invisible.
The waker uses stdlib `log/slog` (the identity plane's precedent — its
embed surface takes a `*slog.Logger`), scoped to the waker package;
one-shot commands keep the repo's stderr style. Stated here so the
precedent is a decision, not an accident.

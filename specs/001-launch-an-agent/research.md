# Research — Launch an agent (Phase 0 of the plan)

The technical decisions behind [`plan.md`](plan.md), each with the alternative
that was rejected. Grounded in soulstream's actual vocabulary
(`turn.post`, `work.open/claim/done/abandon`) and subjects
(`SOULSTREAM.TOPICS.OPS/INFO.<path>`, `SOULSTREAM.PERSONA.NOTIFY.<id>`), read
from source.

## D1 — Reuse the stage-2 work vocabulary; soulrealm is the runner. *No new soulstream ops.*

**Decision.** Soulrealm does not invent an execution vocabulary. It acts as the
**runner** persona that soulstream's work extension already names (work.md
stage 4: *"stage 2 plus a runner … claims execution-flavoured work items, runs
them wherever it lives, streams progress as ops, and attaches results back"*).
The workload's lifecycle maps directly onto existing stage-2 ops:

| Lifecycle | soulstream op | Published by |
|---|---|---|
| requested | `work.open` (describes the execution) | runner (soulrealm) |
| started / placed | `work.claim` | runner |
| exited OK | `work.done` | runner |
| exited error / killed | `work.abandon` | runner |

**Why.** It honours *both* constitutions: soulrealm's single-control-plane
(constitution V — the lifecycle is ops on the topic) and soulstream's
smallest-viable rule (work.md: *"if a stage ever needs core changes beyond a
vocabulary, the design is wrong"*). The `work.claim` race rule (first claim in
stream order wins) is exactly what multi-node placement will need later, for
free.

**Rejected.** A dedicated `exec.*` / `work.progress` sub-vocabulary. It would be
speculative for M1.1 — nothing in this slice streams sub-states. A **progress**
op is a real future need (a long job reporting %), but it is a *soulstream*
design addition to work.md, proposed through soulstream's own process **when a
workload needs it** — not now. This slice touches no soulstream source.

## D2 — Two peer identities: the runner and the workload

**Decision.** Two personas act, both peers (constitution II):

1. The **runner** — soulrealm itself, connected to the realm with its own
   persona credentials, publishing the `work.*` lifecycle ops.
2. The **workload** — the agent, handed a *minted, scoped* credential, connected
   as its own persona, publishing its `turn.post`.

**Why.** Attribution stays honest: the agent's turn is the agent's, the
lifecycle record is the runtime's. Neither is privileged.

**Rejected.** Having soulrealm post the agent's turn on its behalf. That would
launder attribution (soulstream explicitly refuses `on_behalf_of`) and collapse
the two identities the slice exists to keep distinct.

## D3 — The minter: NEX's CredVendor shape, realm-semantic scope

**Decision.** `minter` mints a per-workload NATS user: a fresh user nkey, a JWT
whose `Permissions` are scoped to the persona's realm subjects, signed by the
**realm-account signing key soulrealm holds** (`nats-io/jwt/v2` + `nkeys`). The
scope for an agent P participating in topic T:

- **Pub**: `SOULSTREAM.TOPICS.OPS.<T>` (post turns), `_INBOX.>` (replies), and
  `SOULSTREAM.PERSONA.NOTIFY.*` (to mention others).
- **Sub**: `SOULSTREAM.TOPICS.OPS.<T>` and `SOULSTREAM.TOPICS.INFO.>` (follow),
  `SOULSTREAM.PERSONA.NOTIFY.<P>` (its own inbox), `_INBOX.>`.
- **JetStream**: consumer/read API bounded to the `SOULSTREAM` stream (materialise
  the topic). Publishing a turn is a core publish to the OPS subject the stream
  captures.
- Object-store (`soulstream-objects`) access is **not** granted in M1.1 (no
  attachments in this slice); it is added when a workload attaches.

**Why.** Realm-semantic scope from the start is the exact gap stock NEX left
(its `WorkloadClaims` allow only `_INBOX.>`); reimplementing the minter is what
the rebuild bought. SC-003 tests that the credential cannot publish outside this
set.

**Rejected.** A shared/broad credential (violates II); overriding NEX's internal
`WorkloadClaims` (unreachable + the whole reason we did not embed NEX).

## D4 — Credential delivery: direct env for local exec (refines design 0001 §4)

**Decision.** For a **single-node, local** native process that soulrealm
launches itself, the minted credential is written to the child process's
environment / a private temp creds file directly. The **xkey-encrypted-env**
delivery from design 0001 §4 is the mechanism for when a start request travels
over NATS to a node soulrealm does not control (multi-node) — not needed here.

**Why.** Smallest-viable (soulstream constitution's spirit; soulrealm II): there
is no untrusted intermediary between soulrealm and a process it forked on the
same host, so encrypting the env to protect it from the transport is
machinery M1.1 does not need. **This refines design 0001 §4** — propagated back
there: xkey delivery is a multi-node concern, local exec injects directly.

**Rejected.** Building xkey encryption now. It is real and correct for
multi-node; adding it for a local fork is speculative infrastructure.

## D5 — Native backend: os/exec, file:// artifact

**Decision.** `backend/native` supervises the workload as an OS process
(`os/exec`): fetch the artifact (a local `file://` path for M1.1), inject the
creds env, start, map process exit → the runner's `work.done|abandon`, reap the
scratch. `nats://` object-store artifacts and richer isolation are later
backends.

**Rejected.** Pulling in a container/OCI dependency now — that is M1.3's proof,
and III only requires the *declaration* be backend-agnostic today, which it is.

## D6 — The execution work item lives on the target topic

**Decision.** The `work.*` lifecycle ops go on the **same topic T** the agent
participates in, so one follower of T sees both the agent's turns and its
lifecycle.

**Rejected.** A separate per-node "runtime" topic. Cleaner separation, but
splits the workbench for no M1.1 benefit; revisit only if lifecycle noise on
content topics is felt.

## D7 — soulstream is a Go module dependency

**Decision.** Soulrealm imports `github.com/impire-io/soulstream` and uses its
`realm` (connect), `topic` (`OpenWork`/`ClaimWork`/`CompleteWork`/`AbandonWork`,
`PostTurn`, `Materialise`), `record`, and `identity` surfaces rather than
re-deriving the wire format. This is the whole of soulrealm's domain
dependency surface (episode 0003).

**Rejected.** Re-implementing the soulstream record/op layer — pointless
duplication and a drift risk; the library exists and is the contract.

## Open items carried to `/speckit-tasks` / later features

- Exact JetStream permission subjects for stream read (tuned against SC-003).
- The runner persona's own credential provisioning (dev: an `nsc` user; a
  node-identity story for multi-node later).
- A **progress** op — a *soulstream* work.md addition, when a workload needs it.

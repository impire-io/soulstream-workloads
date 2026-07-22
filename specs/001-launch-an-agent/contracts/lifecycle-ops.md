# Contract — lifecycle as soulstream work ops

How soulrealm expresses a workload's life on the topic op-log, using **only**
existing soulstream stage-2 vocabulary (D1). This is the single control plane
(constitution V). Every op is published by the **runner** persona to
`SOULSTREAM.TOPICS.OPS.<topic>` through the soulstream client — never a
soulrealm-private subject (SC-002 audits for one).

## The sequence

```
work.open      ─ runner ─▶  "execution requested: run <artifact> as <persona>"
   │
work.claim     ─ runner ─▶  the runner takes it — this marks STARTED
   │                        (first-claim-in-stream-order-wins; free multi-node arb later)
   │
   │   … the workload runs; the agent independently posts turn.post as ITS persona …
   │
work.done      ─ runner ─▶  clean exit (code 0)
  or
work.abandon   ─ runner ─▶  non-zero exit, signal/kill, or start failure
```

## Op payloads (functional)

Payloads follow the soulstream record (headers carry the record; payload is
data). Fields below are the M1.1 minimum.

| Op | Key payload fields |
|---|---|
| `work.open` | `title` (human line), `role`, `lifecycle`, `persona`, `artifact` (no creds, no secrets) |
| `work.claim` | (standard claim; claimant = runner persona) |
| `work.done` | `exit_code: 0` |
| `work.abandon` | `reason` (`nonzero-exit` \| `signal` \| `start-failed`), `detail` |

The minted credential and its seed **never** appear in any op (constitution I:
soulrealm is not the store; II: secrets stay with the workload).

## Mapping to the spec

| Spec | Satisfied by |
|---|---|
| FR-006 lifecycle as ops, no private subject | the `work.*` sequence above, all on `TOPICS.OPS.<topic>` |
| US2 "see a workload's life as ops" | a follower of the topic materialises the sequence in order |
| SC-002 zero private control traffic | nothing is published outside the realm's documented subjects |
| SC-004 kill → `exited` op + reap | signal exit → `work.abandon` + scratch/cred reaped |
| FR-008 no silent partial start | pre-launch failure → refuse; at most a `work.open`+`work.abandon(start-failed)` pair, never a dangling claim |

## Ordering & idempotency

- `work.open` then `work.claim` are published in stream order by the same runner;
  the claim's payload references the opened item's op-id (soulstream's claim
  mechanic).
- Exactly one terminal op (`work.done` xor `work.abandon`) per execution.
- The mapping function `lifecycle(state, exit) → op` is **pure** and unit-tested
  with no server; the runner performs the publish.

## What is deliberately absent (deferred, not forgotten)

- **Progress** (a running workload reporting sub-states/%) — would be a new
  `work.progress` op, a **soulstream** work.md addition proposed through
  soulstream's process when a workload needs it (D1). Not M1.1.
- **Results as attachments** (`attachment.add`) — when a workload produces
  artefacts. Needs object-store scope in the minted cred; added with the first
  result-producing workload.

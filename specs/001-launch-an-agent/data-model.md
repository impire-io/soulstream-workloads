# Data model — Launch an agent

Entities for M1.1. Fields are functional (what must exist), not final Go
struct tags. Pure-data entities carry no NATS import.

## WorkloadDeclaration

The operator-facing contract (`declaration` package). Parsed from a file;
validated before anything is minted or launched.

| Field | Type | Notes |
|---|---|---|
| `role` | enum `agent` \| `tool` | M1.1 accepts `agent` only |
| `lifecycle` | enum `service` \| `function` \| `job` | M1.1 accepts `service` only |
| `persona` | string | the realm persona this workload runs as; must be a valid soulstream persona slug (`identity` validation) |
| `topic` | string | the topic path the agent participates in and the lifecycle work item is opened on |
| `artifact` | URI | `file://` for M1.1; the executable to run |
| `args` | []string | optional argv passed to the artifact |

**Invariants** (validation, all pure):

- `role`/`lifecycle` within the accepted M1.1 subset, else a clear rejection.
- `persona` and `topic` pass soulstream slug validation.
- `artifact` scheme is `file://` (M1.1).
- **No backend-specific field is present** — an unknown/backend key is rejected
  (constitution III; SC-005). Strict decode: unknown fields fail loud.

## PersonaScopedCredential

Produced by `minter` for the workload persona; never persisted by soulrealm
beyond the child process's lifetime.

| Field | Type | Notes |
|---|---|---|
| `userJWT` | string | user JWT, `IssuerAccount` = the realm account, `Name` = persona |
| `userSeed` | []byte (secret) | the user nkey seed; delivered to the workload, never logged |
| `natsServers` | []string | how the workload reaches the realm |
| `permissions` | PermissionSet | the scoped pub/sub allow-lists (see below) |
| `expires` | timestamp | bounded lifetime (constitution II / FR-010) |

### PermissionSet (pure; the scope for agent P on topic T)

- **pub allow**: `SOULSTREAM.TOPICS.OPS.<T>`, `SOULSTREAM.PERSONA.NOTIFY.*`,
  `_INBOX.>`
- **sub allow**: `SOULSTREAM.TOPICS.OPS.<T>`, `SOULSTREAM.TOPICS.INFO.>`,
  `SOULSTREAM.PERSONA.NOTIFY.<P>`, `_INBOX.>`
- **JetStream**: read/consumer API bounded to the `SOULSTREAM` stream
- **denied by omission**: everything else (SC-003 asserts a publish outside this
  set is refused by the server)

## ExecutionWorkItem (a view, not a new type)

Not a new soulstream entity — it *is* a stage-2 work item on topic T, created
and advanced by the runner. Tracked here only to name the mapping (full detail
in [`contracts/lifecycle-ops.md`](contracts/lifecycle-ops.md)):

| Lifecycle state | soulstream op on `SOULSTREAM.TOPICS.OPS.<T>` |
|---|---|
| requested | `work.open` (payload: what is being run, as which persona) |
| started | `work.claim` (by the runner) |
| exited OK | `work.done` |
| exited error / killed | `work.abandon` |

## NativeProcess (runtime handle, `backend/native`)

Ephemeral; never leaves the node.

| Field | Type | Notes |
|---|---|---|
| `pid` | int | the OS process |
| `scratchDir` | path | private working dir, reaped on exit |
| `credsPath` | path | temp creds file for the child, `0600`, removed on exit |
| `exit` | status | code/signal → maps to `work.done` vs `work.abandon` |

## Identity flows (two peers)

- **Runner** = soulrealm, connected with its own realm persona creds; publishes
  the `work.*` ops.
- **Workload** = the agent, connected with its `PersonaScopedCredential`;
  publishes `turn.post`.

Both are ordinary soulstream personas; nothing is a privileged tier
(constitution II).

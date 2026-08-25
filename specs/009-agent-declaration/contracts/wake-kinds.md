# Contract: the four wake kinds

The wake engine (`wrap` package) generalizes the measured mention protocol
(specs/006, design 0004) to four declared kinds. Everything here is
normative; the trigger-identity strings are frozen the way `WakeOpID`'s
UUIDv5 namespace is frozen — outcome ids already in records depend on
them.

## The one identity rule

Every wake has exactly one outcome slot:

```
outcome op id = WakeOpID(trigger identity, persona)
             = UUIDv5(wakeNamespace, trigger + "/" + persona)
```

`wakeNamespace` is `wrap/correlate.go`'s existing constant and never
changes. The persona is part of the identity (one trigger can wake several
agents). Outcomes post with `PostTurnIdempotent`, so the id doubles as
`Nats-Msg-Id` and the stream's duplicate window absorbs near reposts; the
outcome-existence check against the materialised topic covers every longer
distance. No durable consumer anywhere: **the record is the position.**

## Per kind

| kind | trigger identity (the exact string hashed) | delivery class | catch-up / position | outcome placement | failure self-report |
|---|---|---|---|---|---|
| `mention` | the notify op id — the mentioning op's id as carried in the notify payload (**unchanged**; pre-009 outcome ids are preserved) | replay-exact (notify stream, inbox-window bounded: newest `realm.InboxWindow`) | inbox fetch + outcome-existence | the triggering topic | taps only the asker (the mentioning author) — unchanged |
| `topic` | the triggering op's id (UUIDv4 from the ops stream) | replay-exact (ops stream — the whole declared path's history is the backlog) | materialise the path + outcome-existence per matching op; live = subscription on `SOULSTREAM.TOPICS.OPS.<path>` | the triggering topic (the declared path) | taps the triggering op's author |
| `schedule` | the tick's SOULSTREAM_SYSTEM **stream sequence**, formatted as a decimal string (e.g. `"137"`) | replay-exact, TTL-bounded backlog (per-tick `Nats-TTL`, stamped via the registration's `Nats-Schedule-TTL`) | ordered (ephemeral) consumer over `SOULSTREAM.SYSTEM.TICKS.<persona>.<name>`, DeliverAll + outcome-existence; expired ticks never wake | the declared **home topic** (the declaration's `topic`) | posts on the home topic with **no mentions** — there is no asker |
| `subject` | the **lowercase-hex SHA-256** digest of the message payload bytes | **at-most-once** — a wake arriving while the engine is down is lost; declaring the kind is declaring that honestly | none, by design (plain core-NATS subscription) | the declared **home topic** | posts on the home topic with **no mentions** |

Notes:

- **Identity collapse**: an op that reaches the agent both as a mention
  and through a topic wake hashes the same op id both times — one outcome
  slot, answered once. Correct, not a race.
- **Subject digest**: identical payloads on the subject collapse to one
  wake identity; distinct payloads are distinct wakes. Hex was chosen
  (over base64url) and is frozen here.
- **Schedule sequence**: SOULSTREAM_SYSTEM stream sequences are unique
  across the stream, so the decimal string cannot collide within a realm;
  its shape (no dashes) also cannot collide with op-id UUIDs or hex
  digests.

## The admission seam (every kind, same order)

```
self-skip → outcome-existence pre-check → 0006 budget → invoke → discharge
```

- **Self-skip**: a trigger authored by the declared persona never wakes it.
  For topic wakes this is the normative self-exclusion (enforced at the
  source too — self-authored ops are never enqueued); schedule/subject
  wakes carry no author and cannot match.
- **Budget** (design 0006, built as specs/008): window floor + depth bound
  computed from the outcome topic's view. Non-record triggers are absent
  from the view and walk as chain roots (depth 1 for their outcome); the
  window floor applies fully. Refusals are op-less and loud
  (`wake_refused` with the numbers); nothing is acked away.
- **Discharge**: reply or self-report under the one outcome id, exactly as
  the mention protocol measured.

## Runtime-side reads (the resolved [O])

Every read above — inbox fetch, materialisation, outcome existence, budget
view, tick consumption, instructions materialisation — runs on the
**engine's connection**: wrap uses the agent's own paste-block credential
exactly as today; a fleet dispatcher will use the runtime persona's. The
minter's agent scope stays `$JS.API.INFO`-only; no JS read tails are added
to agent templates (stream-wide tails breach own-prefix confinement, and
template widening is a per-account founding-time migration — the byon
rc.10 lesson). Consequence: a declared topic path must be readable by the
engine's credential; an unreadable path fails loudly at the source, never
silently.

## Prompt fill (the harness contract, grown)

The template prompt's fill map grows two variables; all existing ones are
unchanged:

| var | value |
|---|---|
| `{{KIND}}` | `mention` \| `topic` \| `schedule` \| `subject` (empty-kind legacy wakes fill `mention`) |
| `{{INSTRUCTIONS}}` | the declared instructions lineage tip, materialised at this wake (empty when none declared) |

- `{{AUTHOR}}` is empty for schedule/subject wakes; `{{BODY}}` carries the
  anchoring op's body for record wakes, the tick payload for schedule
  wakes, and the message payload for subject wakes.
- When a declaration carries instructions and the active template's prompt
  lacks `{{INSTRUCTIONS}}`, the declared config appends
  `"\n\n{{INSTRUCTIONS}}"` to the prompt — visible, deterministic, and a
  no-op for undeclared runs.
- Instructions are digest-checked against the artefact tip and held in
  memory only; a materialisation failure fails the wake loudly (no post,
  trigger stays answerable).

## Schedule reconciliation

Reconciling a declaration's schedule entries is publishing, per entry, one
headered message to `SOULSTREAM.SYSTEM.SCHEDULES.<persona>.<name>`:

| header | value |
|---|---|
| `Nats-Schedule` | the declared pattern (`@every <Go duration>` \| `@at <RFC3339 UTC>` \| 6-field cron) |
| `Nats-Schedule-Target` | `SOULSTREAM.SYSTEM.TICKS.<persona>.<name>` |
| `Nats-Schedule-TTL` | the declared `ttl` (Go duration string), when set |

The payload is a legible line (`schedule "<name>" fired (<pattern>)`) —
the server copies it onto every tick, and it becomes the wake's
`{{BODY}}`. Re-publishing on the same subject **replaces** the schedule;
purging the subject **deregisters** it. Ticks are non-record plumbing:
unsigned, replayable, never authoritative — the outcome op is the record.

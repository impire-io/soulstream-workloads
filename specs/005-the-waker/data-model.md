# Phase 1 data model — the waker

No new durable data. Every entity below is either operator configuration
(versioned files), scratch (per-run directories), or transient consumer
state the transport already owns — the substrate boundary holds by
construction (constitution I; spec FR-010).

## Registration (configuration)

One agent as the waker serves it.

| Field | Meaning | Constraints |
|---|---|---|
| `persona` | The agent's persona name — the notify subject tail and the author of replies | required; realm-unique persona |
| `credential` | The wake lane: `sentinel_creds` + `token` (token lane), or `ephemeral: {role, ttl}` (per-run mint) | exactly one lane; token lane is the probe's subject |
| `template` | The invocation template (below) | required |
| `max_deliver` | Retry budget per wake | ≥ 1; default 2 |
| `run_timeout` | Run time budget | > 0; default 150s |

## Invocation template (configuration)

| Field | Meaning | Constraints |
|---|---|---|
| `command` | argv with `{{PROMPT}}`, `{{MCP_CONFIG}}`, `{{RUN_DIR}}` placeholders | required, non-empty |
| `terminal` | Terminal-event mapping (below) | required — a template without one is refused at load (spec FR-004) |
| `mcp_command` / `mcp_env` | The tool door written into the per-run MCP config; empty `mcp_command` = no MCP config generated | optional |
| `prompt` | Prompt template with `{{PERSONA}}`, `{{TOPIC}}`, `{{AUTHOR}}`, `{{OP_ID}}`, `{{BODY}}` placeholders | required |

## Terminal mapping (configuration)

| Field | Meaning |
|---|---|
| `type_field` | Dot-path to the event's type discriminator (`type`, `msg.type`) |
| `terminal_value` | The discriminator value that marks the terminal event |
| `text_field` | Dot-path to the final text |
| `status_field` / `success_value` | Optional status discrimination; absent = any terminal event is success |

## Wake (transient)

One delivery of one notify message. The only state that survives a waker
restart is the transport's own (unacked delivery + delivery count).

```
delivered ──(NumDelivered>1?)── outcome-already-landed? ──yes── ack (idempotent, D3)
    │                                                      no
    ▼                                                      ▼
 probe ── refused (auth) ──────── nak(long)   [no op — the agent cannot speak]
    │  └─ unreachable (transport) nak(short)  [no op — the realm is the problem]
 admitted
    ▼
 materialize context (before-snapshot + anchoring op body)
    ▼
 run harness (fresh dir, sanitized env, run_timeout, process-group kill)
    ▼
 after-snapshot diff:
    ├─ persona posted during run ─────────── outcome: correlated  → ack
    ├─ terminal success with text ─────────── outcome: reply      → post as agent → ack
    └─ failure (died / timeout / no terminal / error status / empty text)
         ├─ NumDelivered < max_deliver ────── nak(retry delay)
         └─ NumDelivered ≥ max_deliver ────── outcome: failure    → post as WAKER → ack
```

## Outcome op (record)

The one turn an admitted wake leaves in the topic.

| Kind | Author | Body | Publish id |
|---|---|---|---|
| reply | the agent's persona | the harness's terminal text | `soulstream-workloads-wake-<notify-op-id>` |
| correlated | the agent's persona (already posted by the harness itself) | — (no waker publish) | — |
| failure | the **waker's** persona | names the agent, the asker, the legible reason, and the delivery count | `soulstream-workloads-wake-<notify-op-id>` |

The publish id is shared between reply and failure deliberately: a wake has
one outcome slot, whichever kind fills it (D3).

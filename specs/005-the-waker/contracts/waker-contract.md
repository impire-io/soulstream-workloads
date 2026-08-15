# Contract: the waker's configuration and wake-protocol invariants

Two audiences: the operator writing a registration file, and any future
implementation (fleet-era claim path included) that must honor the same
invariants. Frozen when the feature lands.

## The registration file (operator contract)

JSON, strict-decoded (unknown fields refused, the declaration precedent).
Top level:

```json
{
  "waker": {
    "context": "realm-ops",
    "realm": "acme",
    "persona": "waker",
    "scratch": "/var/lib/soulstream-workloads/wakes"
  },
  "agents": [
    {
      "persona": "clerk",
      "credential": {
        "sentinel_creds": "/etc/soulstream/sentinel.creds",
        "token": "sit_…"
      },
      "max_deliver": 2,
      "run_timeout": "150s",
      "template": {
        "command": ["claude", "-p", "{{PROMPT}}", "--output-format",
                    "stream-json", "--verbose", "--mcp-config",
                    "{{MCP_CONFIG}}", "--strict-mcp-config",
                    "--allowedTools", "mcp__soulstream__*"],
        "prompt": "You are @{{PERSONA}}… mentioned by @{{AUTHOR}} in \"{{TOPIC}}\" (op {{OP_ID}}): {{BODY}} …",
        "terminal": {
          "type_field": "type",
          "terminal_value": "result",
          "text_field": "result",
          "status_field": "subtype",
          "success_value": "success"
        },
        "mcp_command": "soulstream-mcp",
        "mcp_env": {
          "SOULSTREAM_URL": "nats://…",
          "SOULSTREAM_CREDS": "/etc/soulstream/sentinel.creds",
          "SOULSTREAM_TOKEN": "sit_…",
          "SOULSTREAM_REALM": "acme",
          "SOULSTREAM_PERSONA": "clerk"
        }
      }
    }
  ]
}
```

Rules:

- `waker.context` names the NATS context carrying the waker's own
  credential (consumer creation + its persona's posting rights). The
  waker's standing is operator-provisioned, never minted (plane
  machinery, like the runner).
- `credential` carries exactly one lane: `{sentinel_creds, token}` (token
  lane) **or** `{ephemeral: {role, ttl}}` (per-run mint; `ttl` ≥
  `run_timeout`). Both present or both absent is a load error.
- `template.terminal` is required; a template without a machine-readable
  terminal mapping is refused at load. Placeholders: `{{PROMPT}}`,
  `{{MCP_CONFIG}}`, `{{RUN_DIR}}` in `command`; `{{PERSONA}}`,
  `{{TOPIC}}`, `{{AUTHOR}}`, `{{OP_ID}}`, `{{BODY}}` in `prompt`.
- `mcp_command` empty ⇒ no MCP config is generated (a harness may act
  through nothing but its reply).
- Durations are Go syntax (`"150s"`). Defaults: `max_deliver` 2,
  `run_timeout` `150s`.

## Wake-protocol invariants (implementation contract)

1. **Consumer**: durable, AckExplicit, filtered to
   `SOULSTREAM.PERSONA.NOTIFY.<persona>` on the `SOULSTREAM_NOTIFY`
   stream; `AckWait` strictly greater than `run_timeout` plus posting
   margin; no server-side MaxDeliver (the retry budget is policy, its
   exhaustion visible as an op, never a silent drop).
2. **Ack after outcome, always**: a delivery is acked only when an
   outcome op exists (posted now, posted by the harness during the run,
   or found by the redelivery pre-check). Every other path naks:
   refused (long delay), unreachable (short delay), retry-budget
   remaining (retry delay).
3. **One outcome slot per wake**: the outcome op id is UUIDv5 of the
   notify op-id, shared by reply and failure. Within the record's
   2-minute duplicate window the id dedupes server-side; beyond it, the
   pre-check on `NumDelivered > 1` (materialise, find the id) closes the
   crash-after-post window. Exactly one op per admitted wake, at any
   redelivery distance.
4. **Authorship is mechanical**: replies post through a client bound to
   the agent's persona; failure testimony posts through the waker's own
   client, mentioning the agent and the asker. There is no code path
   that posts as a persona other than its client's.
5. **Refused wakes are silent on the record and loud in the log**: no op
   of any kind (the agent cannot speak; the waker must not speak for
   it), a structured log line always.
6. **Non-mention notify types**: acked without a wake, logged. The
   notify subject stays general; the waker consumes only what it
   understands.
7. **Run hygiene**: fresh run dir per delivery under the scratch root;
   child env contains no inherited `SOULSTREAM_*`; stdin closed;
   timeout kills the process group; the event stream (not the exit
   code, not prose) decides the outcome.
8. **Bounded backlog is protocol, not policy**: the notify stream keeps
   the newest `realm.InboxWindow` (100) per persona. A waker down long
   enough to overflow that window loses the oldest mentions — by the
   record's design (notifications are pointers; the mentioning ops stay
   in topic history). Documented, not worked around.

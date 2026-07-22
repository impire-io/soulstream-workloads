# Episode 0005 — A tool answers (2026-07-22)

Feature 002 (`specs/002-call-a-tool/`) is done: soulrealm launches a **tool** —
a persistent capability — and an **agent** discovers it by name and calls it,
under the same one-identity model as M1.1. All four success criteria are met
and the whole gate is green `[measured]`.

**What was built**, extending M1.1 rather than replacing it:

- `declaration` accepts `role: tool` (with `lifecycle: service`).
- `minter` grew **role-aware scopes**: an agent may additionally *call* tools; a
  tool gets a *serve-only* scope (subscribe its own service subject, publish
  replies) and nothing else.
- `runner` learned **launch/stop**. A service does not self-exit, so `Run`
  (run-to-completion) split into `Launch` (returns a `Running`), `Wait` (await
  self-exit → terminal op, the agent/job path), `Stop` (terminate + record
  `work.done`, the intentional-service-stop path), and `Serve` (the CLI helper).
  `Run` is now `Launch`+`Wait` — M1.1 behaviour unchanged.
- `cmd/tool-upper` (the reference tool) and a `cmd/soulrealm` that serves a
  workload until signalled.

**The measured lesson** `[measured]`: the first tool call came back with a
*JetStream ack* (`{"stream":"SOULSTREAM","seq":5}`), not the tool's reply. The
soulstream stream captures `SOULSTREAM.>` — including `SOULSTREAM.SVC.*` — so a
request published there is stored and JetStream acks it, racing (and beating)
the tool's own reply. Tool request-reply is **transient, not a stored op**, so
it does not belong on the stream's subjects at all. Moved tool serving to
soulrealm's own `SOULREALM.SVC.*` namespace, which the stream deliberately does
not capture. This is the honest boundary: soulstream's stream carries ops;
soulrealm's transient RPC rides its own subjects on the same account.

**Discovery** is by name: a tool serves on `SOULREALM.SVC.<persona>`, and a
caller derives that subject from the tool's name and requests it — no responders
means "not found," no registry. Smallest-viable; richer discovery (capability
search, health) is later.

**Verified:**

- **SC-001** (discover + call, uppercase round trip) — PASS, integration.
- **SC-002** (tool lifecycle open/claim at launch, done at stop) — PASS,
  integration.
- **SC-003** (tool and agent credentials enforced to their scopes) — PASS,
  operator-mode tests: a tool is denied publishing topic ops; the M1.1 agent
  denial still holds.
- **SC-004** (stop reaps; crash → abandon) — the stop→done and reap paths are
  proven (runner + native tests); the crash→abandon mapping is unit-tested.

Reversal condition: none — records a build.

Trail: `specs/002-call-a-tool/` (spec, plan); packages `declaration`, `minter`,
`runner`, `cmd/tool-upper`, `cmd/soulrealm`, `integration`. Commit 83729a9.

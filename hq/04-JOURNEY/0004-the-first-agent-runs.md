# Episode 0004 — The first agent runs (2026-07-22)

Feature 001 (`specs/001-launch-an-agent/`) is implemented: soulrealm now
launches an agent persona as a native process, mints it a scoped credential,
and it participates in a real soulstream topic — with its lifecycle recorded as
ordinary work ops on that topic. The Go module
`github.com/impire-io/soulrealm` exists, and the whole quality gate (`make fmt
&& make tidy && make build && make test && make lint`) is green with nothing
skipped `[measured]`.

**What was built** — six packages, each with the pure logic split from the I/O
so most tests need no server:

- `declaration` — strict-decoded workload contract; rejects any backend key
  (SC-005) and enforces the M1.1 subset. Pure.
- `minter` — `Scope.PermissionSet` (realm-semantic scope) and `SigningKeyMinter`
  (a fresh NATS user per workload, JWT signed by the realm-account key — NEX's
  CredVendor shape). Pure claim-building; unit-tested that the JWT validates and
  the permissions match the scope.
- `backend` + `backend/native` — the isolation seam and its os/exec
  implementation. It builds a **clean child env** so soulrealm's own secrets
  (the signing key) never leak into a workload — proven by a test that plants a
  parent secret and asserts the child cannot see it `[measured]`.
- `runner` — the "runner" persona: `preflight → work.open → start → work.claim
  → work.done|abandon`. The op sequences (clean / nonzero / start-fail /
  mint-fail) are unit-tested with fakes, including no-dangling-claim and
  no-partial-start (FR-008).
- `cmd/soulrealm` (`workload start`) and `cmd/agent-echo` (the reference agent).

**The plan's headline bet held** `[measured]`: the slice needed **no new
soulstream vocabulary**. The runner drove an execution work item with the
existing `work.open/claim/done`, and `agent-echo` posted an ordinary
`turn.post` — the end-to-end integration test (in-process realm) confirms a
turn attributed to `researcher` and a work item driven to `done` by the runner,
both on the one topic (SC-001, SC-002).

**What is verified, honestly:**

- **SC-001** (turn attributed to the persona) — PASS, integration test.
- **SC-002** (lifecycle as ops) — PASS for the positive half (the work item
  appears and completes on the topic). The negative half ("zero soulrealm-
  private control traffic") is true *by construction* — no code publishes to
  any non-soulstream subject — but is not yet asserted by a test.
- **SC-004** (kill → abandon + reap) — proven at the unit level (native
  signalled-exit → `ExitStatus.Signal`; runner nonzero → `work.abandon`;
  scratch reaped on `Wait`). A live kill-mid-integration test is not written.
- **SC-005** (reject backend field) — PASS, declaration unit test.
- **SC-003** (the minted credential cannot publish outside its scope) — **now
  PASS** (closed same day, commit b696c82). `internal/natstest.StartOperator`
  brings up an in-process operator-mode server (operator + account + signing
  key + resolver + system account) that enforces user JWT permissions;
  `TestMintedCredentialScopeEnforced` proves a minted credential may publish on
  its topic's ops subject but is denied any subject outside its scope. Scope is
  now *enforced*, not just constructed. **All five success criteria met.**

**What it opened / refined.** Design 0001 §4 was refined during the plan: for a
local single-node fork, creds inject directly into the child env; the
xkey-encrypted delivery is the multi-node concern (propagated back to the
design doc). The pattern of soulrealm-as-runner over the unchanged soulstream
work vocabulary is now real code, not just a claim.

Reversal condition: none — records a build. (The SC-003 verification gap is a
tracked follow-up, not a direction decision.)

Trail: `specs/001-launch-an-agent/` (spec, plan, research, data-model,
contracts, tasks, quickstart); packages `declaration`, `minter`, `backend`,
`backend/native`, `runner`, `cmd/soulrealm`, `cmd/agent-echo`,
`internal/natstest`, `integration`. Commits e3ae520, 1293b40, 3b055fd,
e93941a.

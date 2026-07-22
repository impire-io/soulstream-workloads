# Episode 0002 — The substrate decision: a from-scratch, NEX-influenced runtime (2026-07-22)

The `nex-runtime-substrate` research topic asked whether NEX is the right
execution substrate and whether `agent`/`tool` is the right way to model
workloads on it. It graduated to design with a direction call: **soulrealm
builds its own runtime, influenced by NEX but not depending on it, with the
soulstream topic op-log as the single control plane.**

**What was measured** (spike 1: a live `nex node up` on the local dev build
`0.0.0` + a read of its exact source at `~/Work/nex`):

- **Role is orthogonal to lifecycle** `[measured]`. NEX exposes two native
  axes — `--type` (runtime) and `--lifecycle` (service | function | job) — and
  neither is a role axis. A `tool` legitimately spans lifecycles, so `agent`/
  `tool` is a real independent axis, soulrealm's to define (research Bar 3
  PASS).
- **NEX issues scoped per-workload identity for free** `[measured]`. The node
  mints a fresh uniquely-keyed NATS user per workload, delivered via a required
  `workload_creds` field in an xkey-encrypted env — but the stock scope allows
  publishing only `_INBOX.>`, and dev-mode issues nothing.
- **NEX is built to embed** `[measured]`. `NewNexNode(opts…)` plus public
  functional options — `WithMinter(models.CredVendor)`, `WithEventEmitter`,
  `WithAgent`, `WithIDGenerator`, `WithNatsConn` — make every gap spike 1 found
  closable from outside the module, no fork required. Embedding NEX was
  therefore genuinely viable; the permission-scope objection did **not** force
  a fork.

**What was decided, and on what class of evidence.** The measured work closed
the *feasibility* questions but not the *choice*. Three options were live —
embed NEX on top (cheapest, viable), fork it (weakest: upstream-drift cost for
control the options already give), or rebuild NEX-influenced. The deciding
argument is **two control planes**: embedding NEX means permanently running its
control plane (`$NEX.control.*`, auctions, `$NEX.agent.*`) alongside
soulstream's op-log, bridged at a seam. Constitution I puts the op-log as the
one plane the realm trusts. Eliminating the second plane, and fitting the
runtime to soulrealm's specific needs, was judged `[judgment]` to outweigh the
reuse embedding buys. The embed case was argued at full strength first (the
working agreement's adversarial pass); the decision was recorded only after the
maintainer restated the single-control-plane argument in his own words
(teach-back). **Decision class: `[judgment]` on `[measured]` findings** — not a
measured close, and tagged as such.

**What was refuted / reversed.** The topic's own working hypothesis had assumed
we would build *on* NEX and only needed to find the least-invasive layer (Bar
4: metadata > convention > custom nexlet). That frame was set aside: the
measured layers were all reachable, but the decision rejects building on NEX at
all for a reason outside the bar's frame (the second control plane). The
genesis-episode framing of `agent` as a naming *hazard* also softened — not
depending on NEX frees the word from NEX's node-runtime collision, so the role
keeps its natural name.

**What it opened.** Design doc
[`0001-soulrealm-runtime.md`](../02-DESIGN/0001-soulrealm-runtime.md): the
single-control-plane runtime, the role×lifecycle model, a realm-semantic
per-workload minter (NEX's `CredVendor` + xkey-env as influence), lifecycle as
ops over the work.md stage-4 vocabulary, and pluggable isolation backends. It
carries an **influence ledger** so the borrow stays honest, and names the open
sub-questions: the minter's signing/trust story (ties to impire-tenants/vault),
multi-node placement-as-ops, and the Docker/Firecracker/K8s backend interface.

Reversal condition: revert to embedding NEX behind a single-control-plane
adapter if, while building, we find ourselves **reimplementing NEX's execution
layer wholesale** — if the backend + process-supervision + artifact-fetch +
minter machinery reaches rough parity with NEX's nexlet layer in scope and we
maintain it ourselves for no capability NEX lacked. That reading says the
second control plane was the cheaper cost, and the trade should flip.

Trail: `hq/01-RESEARCH/nex-runtime-substrate/` (README verdict + JOURNEY,
removed on graduation — full trail in git history); design
`hq/02-DESIGN/0001-soulrealm-runtime.md`; nex source `~/Work/nex`
(`internal/credentials/{signing_key,vendor,nocreds}.go`, `node.go`,
`options.go`, `models/vendor.go`). Commits <pending>.

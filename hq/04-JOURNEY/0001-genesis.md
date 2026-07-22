# Episode 0001 — Genesis: soulrealm gets an HQ (2026-07-22)

Soulrealm was bootstrapped as a sibling to [soulstream](../../../soulstream):
soulstream is the *record* (topics, ops, baselines, personas), soulrealm is
the *room* — the runtime that launches, supervises, observes, and retires a
realm's agents and tools as workloads. The starting point is soulstream's own
work extension, which defines executable workloads (stage 4) and sandboxes
(stage 5) as coordination *vocabulary* and states the runtime "lives outside
the substrate and is deliberately designed last, against a working stage 4"
[mechanism-argument]. Soulrealm is that deferred runtime.

The hq was seeded from the sibling project's proven structure (GENESIS /
01-RESEARCH / 02-DESIGN / 03-IMPLEMENTATION / 04-JOURNEY, the research→design→
spec-kit→journey pipeline, the anti-drift working agreement). GENESIS is
adapted to a runtime project rather than an ML one: the constitution's
load-bearing article is **the substrate boundary** (soulrealm is never a store
of record; everything worth keeping flows back to topics as ops), with peer
identity, contract/backend orthogonality, research-gates-before-build-spends,
observable execution, and the all-green `make fmt && make test && make lint`
gate. No code exists yet — deliberately, per constitution IV.

The founder's opening architecture question — run agents/tools on **NEX**, the
NATS execution engine, with `agent` and `tool` as workload types — was **not
decided**; it was opened as the first research topic
(`nex-runtime-substrate`, since graduated →
[episode 0002](0002-the-substrate-decision.md))
with four pre-registered bars. Desk research on NEX v0.4.1 found the strong
part of the fit is real [mechanism-argument]: NEX delivers per-workload scoped
NATS credentials (constitution II for free) and a pluggable-nexlet backend
seam that complements container runtimes (constitution III for free). The
working analysis also surfaced a distinction the founder's framing collapses:
`agent`/`tool` is a **role** axis (what a workload is to the realm), while
NEX's native `service`/`function` is a **lifecycle** axis (how it's scheduled)
— a `tool` can legitimately be either lifecycle, so role should not be forced
into a single NEX workload type. And a naming hazard: "agent" is close to NEX's
own node-runtime term (nexlet, formerly "agent"). The topic's Bar 3 tests the
orthogonality claim directly; Bar 4 picks the least-invasive layer that works
(persona metadata > convention > custom nexlet).

Refuted/reversed: nothing yet — this is the first entry. The `agent`/`tool`-
as-NEX-workload-types idea is held as a hypothesis under test, not a decision.

Opened: the substrate gate. Phase 1 (launch one agent, one tool, a second
backend) is sketched in the roadmap but designed only once the gate closes.

Reversal condition: none — records the project's founding and the opening of
the substrate research topic. (The direction bets themselves carry their own
reversal conditions in `constitution.md` and the research topic's README.)

Trail: `hq/` (GENESIS, the nex-runtime-substrate research topic, roadmap);
soulstream `hq/02-DESIGN/extensions/work.md`. Commits <pending>.

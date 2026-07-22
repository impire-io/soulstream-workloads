# 04-JOURNEY — the narrative record

What was built, what was measured, what was believed and then refuted, and
what each episode taught. Specs say what the system *is*; these episodes say
how we *got here* — including the dead ends, because the refuted hypotheses
are as load-bearing as the shipped code.

> **Keeping this log alive:** whenever a feature lands, a research
> investigation concludes, or a load-bearing decision is made, add a numbered
> episode with `/journey-log` (research topics get theirs via
> `/research-graduate`). Follow [`TEMPLATE.md`](TEMPLATE.md) — including its
> required Reversal-condition line and evidence-class tags. Honesty rules
> apply here as everywhere: record what actually happened, including failures,
> reversals, and findings that contradicted expectations. This duty is
> anchored in `../00-GENESIS/how-we-work.md`.

## Where things stand (2026-07-22)

**The project was founded** ([episode 0001](0001-genesis.md)): soulrealm is the
runtime companion to soulstream — soulstream records, soulrealm runs. The hq is
bootstrapped from the sibling project's proven structure, with a constitution
adapted to a runtime project whose non-negotiable article is the **substrate
boundary** (soulrealm never becomes a store of record; everything worth keeping
flows back to topics as ops). No code exists yet, by design (constitution IV):
the runtime is specified last, against decided research.

**The one open gate is the substrate question**
([`01-RESEARCH/nex-runtime-substrate`](../01-RESEARCH/nex-runtime-substrate/README.md)):
is NEX the right execution substrate, and is `agent`/`tool` the right way to
model workloads on it — as a NEX workload type, a convention over NEX's native
service/function workloads, or soulstream persona metadata? Desk research
confirms NEX gives per-workload scoped credentials and a pluggable backend seam
for free; the open question is the modelling layer, with the working hypothesis
that role (`agent`/`tool`) is orthogonal to NEX's lifecycle axis
(`service`/`function`) and should live at the least-invasive layer. Four bars
are pre-registered; no spike has run yet. Everything in the roadmap's Phase 1
is gated on this closing.

## Episode index

| # | Episode |
|---|---|
| 0001 | [Genesis: soulrealm gets an HQ](0001-genesis.md) |

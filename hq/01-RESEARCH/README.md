# 01-RESEARCH — active investigations

One folder per open research topic. Each folder is a self-contained
investigation: a `README.md` (from [`TEMPLATE.md`](TEMPLATE.md)) stating the
question, its state, and the **pre-registered bars**, plus a `JOURNEY.md`
recording the investigation as it happens, plus whatever documents or spikes
the work produces.

## Lifecycle

```
/research-start <slug>            state: active
        │
        ▼
  investigate (pre-registered bars → cheap discriminating
  experiments/spikes → honest verdict; scratchpad for throwaway
  code, repo for conclusions; commit and push as you go)
        │
        ▼
/research-graduate <slug> --to design | artifact | abandoned
        │
        ├─ always: composes the topic's journey into the next-numbered
        │          hq/04-JOURNEY/ episode (verdict, evidence tags,
        │          reversal condition)
        ├─ design:   creates/updates the hq/02-DESIGN doc
        ├─ artifact: the deliverable itself ships (spike, tool, doc)
        └─ always: the topic folder is REMOVED — git history keeps the trail
```

## Rules

- **States are exactly** `active`, `graduated`, `abandoned` — and no folder
  with a terminal state lingers here.
- **Always committed and pushed**, including work heading for abandonment.
  Abandoned research is a result: it gets the same quality of episode as a
  success, and its full history survives in git after the folder is gone.
- **Bars before experiments.** The pass/fail criteria are written down before
  any run; if a bar proves degenerate it is amended openly with the raw
  numbers recorded (constitution / working agreement).
- **Research never goes through spec-kit** — see
  [`../00-GENESIS/how-we-work.md`](../00-GENESIS/how-we-work.md).

## Active topics

*None open.* The `nex-runtime-substrate` topic graduated to design on
2026-07-22 (journey [episode 0002](../04-JOURNEY/0002-the-substrate-decision.md)
→ design [`0001-soulrealm-runtime.md`](../02-DESIGN/0001-soulrealm-runtime.md));
its folder was removed, with the full trail in git history.

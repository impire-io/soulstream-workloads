# 03-IMPLEMENTATION — what gets built, in what order

| File | Role |
|---|---|
| [`roadmap.md`](roadmap.md) | The live plan: phases, milestones, exit criteria, and the research gate each milestone depends on. No dates — gates, not calendars. |

## Conventions

- **Roadmap ↔ journey ↔ specs mapping:** a roadmap item is built as a numbered
  feature through the spec-kit flow (`/speckit-specify` → plan → tasks →
  implement, artifacts frozen in `specs/NNN-*/`), and lands together with a
  numbered episode in [`../04-JOURNEY/`](../04-JOURNEY/README.md). The roadmap
  item links its episode(s) when it closes; feature numbers come from git
  branches, episode numbers from the journey sequence.
- **Landing a feature means, in the same merge:** quality gate green
  (constitution VI), the roadmap item updated with the measured outcome, the
  journey episode written (`/journey-log`), and behavioral changes propagated
  into the [`../02-DESIGN/`](../02-DESIGN/README.md) docs they touch.
- **Exit criteria are written before the work** and amended only openly. The
  roadmap file itself is load-bearing: changes to it are decisions and belong
  in the journey as episodes like any other.

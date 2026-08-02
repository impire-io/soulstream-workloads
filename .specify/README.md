# .specify — spec-kit scaffolding

The full spec-kit setup for soulrealm, vendored to match the sibling `pra`
project so the whole `/speckit-*` flow runs here: specify → clarify → plan →
tasks → implement, plus analyze / checklist / taskstoissues and the git
branching helpers.

- [`memory/constitution.md`](memory/constitution.md) — a **symlink** to
  `../soul-hq/00-GENESIS/constitution.md`, so every plan's Constitution Check reads the
  canonical articles (the pattern `../soul-hq/00-GENESIS/how-we-work.md` commits to).
  The templates keep the generic `[Gates determined based on constitution
  file]` placeholder rather than a baked-in copy, so the check always reflects
  soulrealm's own constitution — not any other project's.
- [`templates/`](templates/) — the generic spec / plan / tasks / checklist /
  constitution templates.
- [`scripts/bash/`](scripts/bash/) — the workflow scripts the `/speckit-plan`,
  `/speckit-tasks`, and `/speckit-implement` skills invoke (`setup-plan.sh`,
  `setup-tasks.sh`, `check-prerequisites.sh`, `create-new-feature.sh`,
  `common.sh`). They locate the repo root by the `.specify/` marker, so they
  carry no per-project paths.
- [`extensions/`](extensions/) — the git branching extension (feature-branch
  numbering, repo init, auto-commit) and `extensions.yml`, the hook registry
  the speckit skills consult before and after each phase.
- [`workflows/`](workflows/) and [`integrations/`](integrations/) — the bundled
  SDD workflow and the claude / speckit install manifests.

Feature specs live in `specs/NNN-<slug>/` at the repo root. `feature.json` is
written by `/speckit-specify` when a feature begins — it is per-feature state,
not vendored, so it is absent until the next feature starts. Authoring still
follows `../soul-hq/00-GENESIS/how-we-work.md`: research never goes through spec-kit,
implementation always does.

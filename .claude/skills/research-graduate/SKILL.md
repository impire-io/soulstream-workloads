---
name: "research-graduate"
description: "Close a research topic (design | artifact | abandoned): compose its journey episode, create/update the design doc when graduating to design, remove the topic folder."
argument-hint: "<slug> --to design|artifact|abandoned"
compatibility: "Requires the hq/ structure (hq/01-RESEARCH, hq/02-DESIGN, hq/04-JOURNEY)"
metadata:
  author: "soulrealm-hq"
user-invocable: true
disable-model-invocation: false
---

## User Input

```text
$ARGUMENTS
```

Parse `$ARGUMENTS` as `<slug>` and `--to design|artifact|abandoned`. Both are
required; if the outcome is missing, ask — do not infer it.

## Steps

1. **Load the topic.** `hq/01-RESEARCH/<slug>/README.md` and its `JOURNEY.md`
   must exist and State must be `active`; otherwise stop and report. Read the
   pre-registered bars and the topic journey in full.

2. **Verdict first.** Fill the topic README's Verdict section: PASS/FAIL per
   pre-registered bar with the honest findings, each load-bearing claim tagged
   `[measured]` / `[mechanism-argument]` / `[judgment]`. If a bar was amended
   during the work, the amendment and the raw findings that forced it must
   already be in the topic journey — if they aren't, stop and reconstruct
   honestly with the user.

3. **Compose the episode** — never a raw file move. Determine the next free
   episode number `NNNN` in `hq/04-JOURNEY/` and write
   `hq/04-JOURNEY/NNNN-<slug>.md` following `hq/04-JOURNEY/TEMPLATE.md`: the
   question, the bars and their verdicts with findings, what was refuted or
   reversed, what it taught or opened, evidence-class tags, and a **Reversal
   condition:** line (for an abandoned topic: what evidence would reopen it;
   this line is required). Fold in the topic journey's substance; link the
   trail documents.

4. **Route the outcome.**
   - `design`: create the next-numbered `hq/02-DESIGN/NNNN-<slug>.md` (or
     update the existing doc it amends) — functional level, explicit enough
     for `/speckit-specify`. The episode links it.
   - `artifact`: the deliverable ships wherever it belongs (a spike promoted
     under `examples/`, a `cmd/` tool, a doc); the episode links it.
   - `abandoned`: nothing ships; the episode is the record.

5. **Update the index.** Add the episode to the index table in
   `hq/04-JOURNEY/README.md` and refresh its "Where things stand" section.
   Remove the topic from the "Active topics" table in
   `hq/01-RESEARCH/README.md`. If the outcome closes or reshapes a roadmap
   item, update `hq/03-IMPLEMENTATION/roadmap.md` accordingly.

6. **Remove the topic folder** — on **every** outcome, including graduation
   (`git rm -r hq/01-RESEARCH/<slug>/`). Git history keeps the full trail; a
   lingering terminal-state folder is illegal.

7. **Gate, commit (never push).** Run the full quality gate (`make fmt && make
   test && make lint`); the hq structural lint, `internal/hqlint`, rides it.
   Stage only the touched paths by explicit pathspec (never
   `git add .`/`-A`); signed commit: `research(<slug>): graduate --to
   <outcome> — <one-line verdict>`, with the standard co-author trailer.
   Remind the human to push.

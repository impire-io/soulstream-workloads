---
name: "journey-log"
description: "Append a numbered journey episode for a completed feature, concluded investigation, or load-bearing decision; refresh the index, Where-things-stand, and roadmap."
argument-hint: "What happened (feature landed / decision made), or empty to log the work just completed in this session"
compatibility: "Requires the hq/ structure (hq/04-JOURNEY/TEMPLATE.md)"
metadata:
  author: "soulrealm-hq"
user-invocable: true
disable-model-invocation: false
---

## User Input

```text
$ARGUMENTS
```

If `$ARGUMENTS` is empty, the subject is the work completed in the current
session; reconstruct it from the conversation and `git log`.

## Steps

1. **Scope check.** An episode is warranted for: a landed feature, a concluded
   investigation (research topics get theirs via `/research-graduate` instead
   — do not duplicate), or a load-bearing decision (spec change, criterion
   amendment, refuted hypothesis, direction call). If none applies, say so and
   stop.

2. **Write the episode.** Next free number `NNNN` in `hq/04-JOURNEY/`, file
   `NNNN-<short-kebab-slug>.md`, following `hq/04-JOURNEY/TEMPLATE.md` exactly:
   what happened with the honest findings, what was refuted or reversed, what
   it taught or opened, evidence-class tags (`[measured]` /
   `[mechanism-argument]` / `[judgment]`) on load-bearing claims, the trail
   (docs, commits), and the required **Reversal condition:** line. For
   direction decisions the working agreement applies first: teach-back before
   recording, adversarial pass for vision-level calls
   (`hq/00-GENESIS/constitution.md`, The Working Agreement).

3. **Refresh the surroundings, same change-set:** add the episode to the index
   in `hq/04-JOURNEY/README.md`; refresh its "Where things stand" section;
   update the affected item in `hq/03-IMPLEMENTATION/roadmap.md`; propagate
   behavioral changes into the `hq/02-DESIGN/` docs they touch.

4. **Gate, commit (never push).** Full quality gate green (`make fmt && make
   test && make lint`; the hq structural lint, `internal/hqlint`, rides it). Stage
   only the touched paths by explicit pathspec (never `git add .`/`-A`); signed
   commit — ideally amend/join the commit of the work the episode describes,
   otherwise `journey(NNNN): <title>` with the standard co-author trailer.
   Remind the human to push.

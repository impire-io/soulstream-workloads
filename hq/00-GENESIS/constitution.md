# Soulrealm Constitution

The canonical copy of this file lives at `hq/00-GENESIS/constitution.md`; once
the project enters implementation, `.specify/memory/constitution.md` is a
symlink to it, so every spec-kit plan's Constitution Check reads these
articles. Decisions are held against this file and [`vision.md`](vision.md) —
see the decision test in [`README.md`](README.md).

## Core Principles

### I. The Substrate Boundary (NON-NEGOTIABLE)

Soulrealm is a runtime, never a store of record. The authoritative home of any
artefact — its bytes, its history, its current state — is the soulstream
topic: object store for the bytes, ops for the history, baseline for the
state. Soulrealm launches, supervises, observes, and retires workloads;
everything worth keeping flows back into the topic as ops. A workload that
dies loses scratch state, never history. No feature may make soulrealm the
place a piece of durable truth lives. This article does not relax for
convenience.

### II. One Identity, No Privileged Tier

Every workload runs as a persona with scoped NATS credentials — the same kind
of identity a human persona holds. There is no bot API and no elevated
standing for machine personas; agents and tools are peers on the wire,
addressed and attributed like any other persona (soulstream's peer principle,
extended to execution). Credentials are always scoped to what a workload needs
and no more. Behaviour may never branch on whether a persona is human or
machine.

### III. Contracts Orthogonal to Backends

A workload *contract* (what a workload is and how the realm interacts with it)
is defined independently of the isolation *backend* that runs it (native
process, Docker/OCI, Firecracker, Kubernetes). A contract must never leak
which backend is in use, and a backend must be swappable per node without
touching a single workload declaration. Where the two axes meet — resource
limits, credential injection, lifecycle signals — the seam is explicit and
documented. If a contract can only be satisfied by one backend, the design is
wrong.

### IV. Research Gates Before Build Spends

The runtime is deliberately designed last, against a real coordination
vocabulary — not ahead of it. Every build milestone names the research gate it
depends on (a decided substrate, a proven contract, a real stage-4 need in
soulstream), and no runtime machinery is built ahead of the vocabulary it
serves. Speculation about a future runtime is research, recorded in
`01-RESEARCH/`; it is not code.

### V. Execution Is Observable and Attributable

A workload's lifecycle — start, progress, result, exit — is visible in the
realm and attributable to a persona, or the work is not done. Execution is not
a black box that occasionally emits a result; it is a stream of operations
anyone in the topic can follow and replay, the same way conversation and
artefacts are. If a runtime backend cannot surface lifecycle as ops, that gap
is a named limitation, written down, not a silent hole.

### VI. All-Green Quality Gate

Done means the full gate is green with nothing skipped: `make fmt && make test
&& make lint` (which includes the hq structural lint, `internal/hqlint`). Tests
that need no NATS server run without one; anything touching NATS uses an
in-process or fake transport so the suite has no external dependency. Sign
every commit. Never commit `.claude/settings.local.json`. Hook or gate
failures are blocking — fixed before anything else continues.

## The Working Agreement (Anti-Drift)

Inherited from the sibling projects' hard-won practice, and applied here from
day one. Applies to every load-bearing decision.

1. **Teach-back as a gate.** No load-bearing direction decision is recorded
   until the maintainer can restate the argument for it in his own words. If
   he can't, the decision isn't ready — the deficit is in the explanation,
   not the listener.
2. **Claims carry their evidence class.** Every load-bearing claim is tagged
   **[measured]** (a reading in the repo), **[mechanism-argument]** (a
   reasoned case, attackable by reasoning), or **[judgment]**. Only measured
   closes a debate.
3. **Decisions record the reversal condition.** Every direction decision gets
   a "what would change our minds" line written *when the decision is made*
   (the journey episode template requires it), so a future reversal is a
   clean, anticipated turn instead of drift.
4. **Adversarial pass on direction changes.** For vision-level calls, the
   other side is argued at full strength before the decision — the maintainer
   never sees only the most convincing case.

## Development Workflow

Work flows through `hq/` as described in [`how-we-work.md`](how-we-work.md):
research (`01-RESEARCH/`, lifecycle active → graduated | abandoned) → design
(`02-DESIGN/`, functional specs explicit enough for `/speckit-specify`) →
implementation (the spec-kit flow specify → plan → tasks → implement on a
numbered feature branch, tracked in `03-IMPLEMENTATION/roadmap.md`) → journey
(`04-JOURNEY/`, one numbered episode per landed feature, concluded research
topic, or load-bearing decision). Research never goes through spec-kit;
designs always do. Every behavioral change propagates into the design docs it
touches.

## Governance

This constitution supersedes all other practices. An amendment requires: the
explicit textual change, a semantic version bump (MAJOR: article removed or
redefined; MINOR: article added or materially extended; PATCH: clarification),
a journey episode recording the why and the reversal condition, and
propagation into any spec-kit template that depends on the changed text.
Spec-kit plans verify compliance through the Constitution Check; reviews call
out violations rather than accommodate them.

**Version**: 0.1.1 (draft — ratifies when the first design graduates) |
**Drafted**: 2026-07-22 | **Amended**: 2026-07-24 (0.1.1, PATCH — the hq
structural lint now exists, so Article VI drops the "once it exists" qualifier;
recorded in [journey 0006](../04-JOURNEY/0006-hq-alignment.md))

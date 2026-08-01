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

## Where things stand (2026-08-01)

**Soulstream is pinned at v0.6.0 — the dev replace is gone** ([episode
0011](0011-pinned-to-the-record.md)): soulnode's composition research named
the filesystem `replace` a release blocker for any consumer wanting to pin
soulrealm; soulstream's tag proved current (main = v0.6.0 + docs only
[measured]) and the whole gate runs green against it. Co-development now
rides an untracked `go.work`; soulstream changes arrive by tag bump.

**The project was founded** ([episode 0001](0001-genesis.md)): soulrealm is the
runtime companion to soulstream — soulstream records, soulrealm runs. The hq is
bootstrapped from the sibling project's proven structure, with a constitution
whose non-negotiable article is the **substrate boundary** (soulrealm never
becomes a store of record; everything worth keeping flows back to topics as
ops).

**The substrate question is decided** ([episode
0002](0002-the-substrate-decision.md)): after a live NEX spike and a source
read, **soulrealm builds its own runtime — NEX as influence, not dependency —
with the soulstream op-log as the single control plane.** Measured: role
(`agent`/`tool`) is orthogonal to lifecycle (`service`/`function`/`job`); NEX
issues scoped per-workload identity for free and is embeddable via public
options (so a fork was never forced). The `[judgment]` call to rebuild rather
than embed turned on **not running a second control plane** (`$NEX.control.*`)
beside the op-log — recorded after teach-back, with the embed case argued at
full strength first. That opened design doc
[`0001-soulrealm-runtime.md`](../02-DESIGN/0001-soulrealm-runtime.md): the
single-plane runtime, the role×lifecycle model, a realm-semantic per-workload
minter, lifecycle-as-ops, and pluggable backends, with an honest NEX influence
ledger and its `[O]` open sub-questions.

**The first slice is specced** ([`specs/001-launch-an-agent/spec.md`](../../specs/001-launch-an-agent/spec.md)):
declare an `agent`/`service`, mint a persona-scoped credential, launch it
native, post a turn as its persona, lifecycle visible as ops — no second
control plane. Minimal spec-kit scaffolding is in place (`.specify/` with the
constitution symlinked). The signing story is resolved soulrealm-held.

**Scope is soulstream-only** ([episode 0003](0003-soulstream-only-scope.md)):
soulrealm depends on soulstream and nothing else — no Impire-platform services
(identity, tenancy, vault) for now. The minter stays a seam for a future
external authority, but none is designed in.

**M1.1 is implemented** ([episode 0004](0004-the-first-agent-runs.md)): the Go
module `github.com/impire-io/soulrealm` exists, and an agent launched by
soulrealm posts a turn attributed to its persona while its lifecycle shows up
as `work.open/claim/done` on the topic — proven end-to-end (SC-001, SC-002),
whole gate green. The plan's bet held: **no new soulstream vocabulary** —
soulrealm is the work.md "runner". Six packages (declaration, minter,
backend/native, runner, two cmds), pure logic split from I/O so most tests need
no server; the native backend proves it does not leak soulrealm's secrets into
a workload. All five success criteria met (SC-003 enforcement via an in-process
operator-mode server).

**M1.2 is done** ([episode 0005](0005-a-tool-answers.md)): soulrealm launches a
`tool` service and an agent discovers it by name and calls it (uppercase round
trip). Added the tool role, role-aware scopes, and the runner's launch/stop
(services don't self-exit). A measured lesson landed a boundary: tool
request-reply is transient, so it rides soulrealm's own `SOULREALM.SVC.*`
subjects, not the stored `SOULSTREAM.>` stream (which would ack and race the
reply). SC-001/002/003 proven end-to-end.

**The hq is now aligned with its own contract** ([episode
0006](0006-hq-alignment.md)): the "hq structural lint" the constitution and
skills had cited as the enforcement backbone is finally built —
`internal/hqlint`, a test-only Go package that rides `make test` and the commit
gate. Along the way: README/CLAUDE status corrected to Phase 1 (M1.1 + M1.2
done); specs 001/002 marked Shipped with 002's spec-kit short-circuit recorded
honestly; the full spec-kit flow vendored from pra (which also fixed a
plan/tasks template that had baked in pra's constitution principles); and
Article VI clarified (constitution 0.1.1).

**M1.3 is done — Phase 1 is complete** ([episode 0007](0007-a-second-wall.md)):
the byte-identical M1.1/M1.2 declarations run inside **microsandbox**
microVMs (an open amendment: the roadmap had said Docker/Firecracker), with
the identical op mapping on the topic and a measured isolation boundary — a
host path readable natively is denied in-guest. The `msb` CLI is supervised
as a child process (no CGO SDK); loopback NATS is rewritten to the guest's
host alias under a host-only network policy; backend selection is node-side
(`SOULREALM_BACKEND`), and the declaration still cannot name a backend. The
default suite stays hermetic (stub CLI); `make test-msb` boots real microVMs.
A refuted diagnosis is on record (":ro unsupported" was really msb failing on
symlinked mount sources), plus a named limitation (remote NATS needs the
`public` profile — Fleet-era).

**The Kubernetes gate is met** ([episode 0008](0008-kubernetes-backend.md)):
the `kubernetes-backend` research topic pre-registered four bars and four
spikes measured all four PASS — a prototype backend behind the *unchanged*
seam ran the byte-identical M1.1/M1.2 declarations as pods on kind (agent,
tool round trip, crash → abandon), and a scope probe inside a pod against
Synadia NGS was denied out-of-scope with its credential delivered as a
Secret. One expectation inverted: on non-loopback NATS, Kubernetes is
*ahead* of the microVM backend (ordinary pod egress vs msb's Fleet-era
`public` profile). Named honestly: pods are weaker isolation than microVMs —
the case is adoption, not isolation. Opened design
[`0002-kubernetes-backend.md`](../02-DESIGN/0002-kubernetes-backend.md).

**M2.1 is done — Phase 2 is complete** ([episode
0009](0009-a-third-wall-lands.md)): `backend/k8s` landed through the full
spec-kit flow — one runner-supervised pod per workload, credential as a
Secret that never touches host disk, artifact as a per-run OCI image on the
CA-trusted base pushed digest-pinned to the operator's registry. Five e2e
scenarios green on kind + a local registry in ~26 s (`make test-k8s`);
default gate hermetic; `make test-msb` still green. Two recorded decisions
from planning: the artifact channel **reversed** from the draft's node HTTP
to an OCI-registry interface (maintainer decision — an open amendment to
design 0002's candidates), and client-go was chosen after teach-back, kept
entirely inside `backend/k8s`. **Next:** research gates for the later
horizons — Fleet, sandboxes (stage 5), tool ecosystem — and the `nats://`
artifact-addressing question at the artifact-registry milestone.

**The Fleet gate is met** ([episode 0010](0010-fleet.md)): the `fleet`
research topic pre-registered three bars and four spikes measured all three
PASS — placement **is** `work.claim` (exactly-one-launch 120/120 contested
rounds, replay-reconstructible, zero transient signaling), node death
reclaims via *projection nominates → probe vetoes → ordinary `work.abandon`
decides* (10/10 kills per variant within bound; the probe eliminates the
live-silent false positive at zero cost on true deaths), and a node without
the signing seed launches a scope-enforced workload (expiry floor measured
at 10 ms). Two open reversals on the record: Bar 2 amended pre-run when
work.md's timeout-by-projection surfaced, and spike 3's judgment against
scoped signing keys fell to spike 4's tag-template measurement — the minter
role dissolves into the identity plane (`soulidentity`), amending episode
0003's soulstream-only scope. Opened design
[`0003-fleet.md`](../02-DESIGN/0003-fleet.md); roadmap Phase 3 (Fleet) is
unblocked. **Next:** the spec-kit pass for the first Fleet milestone; the
soulidentity tags-on-mint addition gates the preferred minting path.

## Episode index

| # | Episode |
|---|---|
| 0001 | [Genesis: soulrealm gets an HQ](0001-genesis.md) |
| 0002 | [The substrate decision: a from-scratch, NEX-influenced runtime](0002-the-substrate-decision.md) |
| 0003 | [Soulstream-only scope: the platform waits](0003-soulstream-only-scope.md) |
| 0004 | [The first agent runs](0004-the-first-agent-runs.md) |
| 0005 | [A tool answers](0005-a-tool-answers.md) |
| 0006 | [HQ alignment: the lint gets built](0006-hq-alignment.md) |
| 0007 | [A second wall: the microsandbox backend](0007-a-second-wall.md) |
| 0008 | [A third wall on rented ground: Kubernetes as a backend](0008-kubernetes-backend.md) |
| 0009 | [A third wall lands: the Kubernetes backend ships](0009-a-third-wall-lands.md) |
| 0010 | [Fleet: the log nominates, evidence vetoes, the log decides](0010-fleet.md) |
| 0011 | [Pinned to the record: the soulstream replace drops](0011-pinned-to-the-record.md) |

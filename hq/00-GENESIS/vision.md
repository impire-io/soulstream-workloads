# Soulrealm Vision

## What Soulrealm is

Soulrealm is the **execution ground of a realm**: the runtime that launches,
supervises, observes, and retires the agents and tools of a
[Soulstream](../../../soulstream) realm as workloads — on a laptop, a server,
or a fleet.

Soulstream is the record; soulrealm is the room. Soulstream gives a realm its
stream — topics as shared workbenches, operations, baselines, personas — and
deliberately stops at the runtime. Its work extension names *executable
workloads* and *sandboxes* as coordination vocabulary and states plainly that
the runtime itself "lives outside the substrate and is deliberately designed
last, against a working stage 4." Soulrealm is that deferred runtime, built
now that the vocabulary it serves exists.

## The founding bet

**A running agent is a persona, not a service tier.** An agent that
participates in a realm holds the same kind of credentials, publishes the same
operation record, and is addressed the same way as any human persona. There is
no bot API. Soulrealm launches personas into a realm and gets out of the way;
what an agent does while it runs is soulstream operations, visible and
attributable like everything else. Everything on the horizon below leans on
this bet — the same one soulstream made about identity, now extended to
execution.

## Who it is for

Realm operators — people running a soulstream realm who want its agents and
tools to actually *run* somewhere, without standing up a Kubernetes cluster or
learning an orchestration stack. Point soulrealm at a realm, declare a
workload, and it runs — on the machine in front of you to start, across a
fleet when you outgrow it. The isolation backend (a bare process, a container,
a microVM) is a deployment choice, never a rewrite.

## Where it is pointed

Horizon ambitions, not schedulable milestones — each shapes design decisions
today and sits behind a named research gate (tracked in
[`../03-IMPLEMENTATION/roadmap.md`](../03-IMPLEMENTATION/README.md)):

- **One workload contract, many backends.** An agent or tool is declared once
  and runs unchanged whether isolated as a native process, a Docker/OCI
  container, a Firecracker microVM, or a Kubernetes pod. The backend is
  pluggable per node; the contract never leaks which one is in use.
- **Tools as first-class realm citizens.** A capability an agent calls — an
  MCP server, a code sandbox, a data connector — runs under the same runtime
  and the same identity model as an agent, discoverable and callable over the
  realm's own transport.
- **The sandbox made physical.** Soulstream's work-extension stage 5: a shared
  environment where artefacts are checked out onto a bench, tools operate on
  them, and outputs flow back as ops. Soulrealm is where that room is built,
  once stage-4 execution is real.
- **Fleet without orchestration.** From one node to many with the same mental
  model — NATS-native scheduling, location transparency, no separate control
  plane to operate.

## What we refuse to become

- **A store of record.** Soulrealm never becomes the authoritative home of an
  artefact, its history, or its state. That lives in soulstream topics. A
  workload that dies loses scratch state, never history. This is the
  substrate boundary, and it is non-negotiable (constitution I).
- **A privileged bot tier.** No special API, no elevated standing for
  machine personas. Agents and tools are peers on the wire.
- **An orchestration platform.** We are not rebuilding Kubernetes. Soulrealm
  complements existing runtimes; it does not replace them. If a deployment
  needs a full orchestrator, soulrealm runs *on* it, not instead of it.
- **A runtime invented ahead of its vocabulary.** No execution machinery ships
  before the soulstream coordination vocabulary it serves is real. Deferred is
  not dismissed; speculative is not built.

## How ambition stays honest

Every ambition above sits behind a named, pre-registered gate, and no design
outruns a decided substrate. Direction decisions record what would change our
minds when they are made, so a future reversal is a clean, anticipated turn
instead of drift. The full discipline lives in
[`constitution.md`](constitution.md) and [`how-we-work.md`](how-we-work.md).

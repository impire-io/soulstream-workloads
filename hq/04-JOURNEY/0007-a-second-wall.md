# Episode 0007 — A second wall: the microsandbox backend (2026-07-28)

M1.3 asked whether the backend seam is real: can a second isolation backend
run the **byte-identical** declarations that ran natively in M1.1/M1.2, with
nothing above the seam changing? It can — **[measured]**, asserted in-test:
one declaration value serialized before each run, compared byte-for-byte, the
node swapping only the artifact build at the declared path (host build for
native, guest build for the sandbox — provisioning, not contract). The agent
posts its persona-attributed turn from inside a microVM with
`work.open/claim/done` on the topic; the tool serves its uppercase capability
from inside its own microVM and the caller's `"hi"` comes back `"HI"`; a
crash inside the sandbox lands as `work.abandon`; and an isolation probe
gives the boundary teeth — a host file readable under the native backend is
**denied** from inside the sandbox **[measured]**. After every end-of-life,
zero sandboxes, scratch dirs, or credentials remain.

The backend is **microsandbox** (libkrun microVMs), an **open amendment** to
the roadmap's design-time "Docker or Firecracker": Firecracker cannot run on
the macOS dev machine at all, Docker-on-mac is one shared daemon VM, and
microsandbox gives each workload its own guest kernel *and* keeps the whole
gate runnable on the operator's laptop (SC-005) **[mechanism-argument]**.
Two implementation decisions carried the slice: soulrealm supervises the
`msb` CLI as its child process — the exact structural mirror of the native
backend, with the guest exit code propagating through it **[measured]** —
rather than adopting the official Go SDK, whose FFI design would force CGO
into every build of the module and whose beta API has already retracted a
release for breaking changes; and the workload env contract is untouched,
only its *values* adapted (creds at an in-guest path; loopback NATS URLs
rewritten to `host.microsandbox.internal`, reachable under a deny-by-default
network policy opened only toward the host **[measured]**). The artifact
enters the guest by pre-boot copy — the host copy is never exposed to the VM.

One diagnosis was refuted along the way: the first probe of read-only mounts
failed and was recorded as ":ro unsupported" — wrong. The real bug, isolated
by controlled comparison **[measured]**, is that msb 0.6.7 cannot mount any
source whose path traverses a symlink (macOS tempdirs live behind `/var →
/private/var`); on resolved paths `:ro` works fine. The backend now resolves
symlinks before handover, the research file records the correction, and the
bug is a candidate upstream report. Named limitation, written down per
constitution V: a NATS server not on the node's loopback would need the
`public` network profile — a Fleet-era concern.

What it opened: Phase 1 is complete — contract, identity, lifecycle, and now
isolation are all proven orthogonal. The hermetic/e2e split earned its keep
(`make test` needs no microsandbox — the backend unit-tests against a stub
CLI; `make test-msb` boots real microVMs, all four scenarios in ~10 s
**[measured]**), and it is the template for every future backend. The later
horizons (Fleet, sandboxes stage 5, tool ecosystem) now wait on their own
research gates.

Reversal condition: if microsandbox's beta churn breaks the CLI seam twice in
one quarter (flag or behavior changes that fail `make test-msb`), or an
upstream stall leaves the symlink/mount class of bugs unfixed for six months,
revisit the backend choice — the seam contract
(`specs/003-microsandbox-backend/contracts/backend-seam.md`) is written so a
Docker/Firecracker implementation can replace it without touching a
declaration.

Trail: [`specs/003-microsandbox-backend/`](../../specs/003-microsandbox-backend/)
(spec, measured research D1–D6, plan, contracts, tasks — full spec-kit flow,
no short-circuit this time); design
[`0001-soulrealm-runtime.md`](../02-DESIGN/0001-soulrealm-runtime.md) §4/§6/§8/§9
amended (including the soulidentity pointer behind the minter seam);
[`roadmap.md`](../03-IMPLEMENTATION/roadmap.md) M1.3 closed; code in
`backend/msb/`, `cmd/soulrealm` (node-side `SOULREALM_BACKEND` selection),
`integration/msb_e2e_test.go`, `Makefile` (`test-msb`); commits on
`003-microsandbox-backend`.

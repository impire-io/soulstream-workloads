# Tasks: Launch an agent — the first runtime slice

**Input**: [`plan.md`](plan.md), [`spec.md`](spec.md), [`research.md`](research.md),
[`data-model.md`](data-model.md), [`contracts/`](contracts/)
**Tests**: included — the spec has explicit success criteria and constitution VI
mandates an all-green gate.
**Organization**: by user story (US1 P1 MVP → US2 P2 → US3 P3), pure/server-free
work first so most of it tests without infra.

Format: `[ID] [P?] [Story] Description` — **[P]** = parallelizable (distinct
files, no dep).

## Phase 1: Setup

- [ ] **T001** Init module `github.com/impire-io/soulrealm` (Go 1.26). Add deps:
  `github.com/impire-io/soulstream`, `nats-io/nats.go` (+ jetstream),
  `synadia-io/orbit.go/natscontext`, `nats-io/jwt/v2`, `nats-io/nkeys`. Use a
  `replace github.com/impire-io/soulstream => ../soulstream` for local dev
  (INTEGRATION UNKNOWN to confirm: whether the needed `topic` work APIs are in a
  published tag; if so, drop the replace).
- [ ] **T002** [P] Repo skeleton per plan (`declaration/`, `minter/`,
  `backend/`, `backend/native/`, `runner/`, `cmd/soulrealm/`) + `Makefile`
  (`fmt`/`test`/`lint`) + `.golangci.yml`. `make lint` green on empty packages.

## Phase 2: Foundational — pure, server-free (BLOCKS all stories)

These import no NATS and unit-test with no server.

- [ ] **T003** [P] `declaration`: `Parse` + `Validate` — strict decode (unknown
  field fails loud → rejects any backend key, SC-005/FR-001), `role`/`lifecycle`
  M1.1 subset, `persona`/`topic` via soulstream `identity` validation,
  `artifact` scheme `file://`. Table tests for each invariant.
- [ ] **T004** [P] `minter`: `Scope.PermissionSet()` (pure) — exact pub/sub
  allow-lists from data-model (`SOULSTREAM.TOPICS.OPS.<T>` etc.). Tests assert
  the set and that unrelated subjects are absent (basis for SC-003).
- [ ] **T005** [P] `runner`: pure lifecycle→op mapping `(state, exit) →
  work.open|claim|done|abandon` per [`contracts/lifecycle-ops.md`](contracts/lifecycle-ops.md).
  Tests: clean exit→`done`, nonzero/signal→`abandon`, exactly one terminal op.

## Phase 3: US1 — Launch an agent that participates (P1, MVP)

- [ ] **T006** `minter.Mint` — build `jwt.UserClaims` from `PermissionSet()`,
  `IssuerAccount`=realm account, `Name`=persona, bounded `Expires`; sign with the
  realm-account signing key. Test: JWT validates under the account; permissions
  match; seed usable.
- [ ] **T007** `backend` interface + `backend/native` `Start`/`Handle` — `os/exec`
  supervision, inject creds into child env (D4 direct injection), private scratch
  dir + `0600` temp creds file, `Wait`→`ExitStatus`, `Stop`, reap. Test with a
  trivial child binary.
- [ ] **T008** `runner.Run` happy path — connect as the runner persona (soulstream
  client), publish `work.open`+`work.claim` on the topic, `minter.Mint`,
  `backend.Start`, on clean exit publish `work.done`, reap. Pre-launch failure →
  refuse + observable error, nothing half-done (FR-008).
- [ ] **T009** `cmd/soulrealm` — `soulrealm workload start <declaration-file>`;
  realm connection via `natscontext`; signing key from env (quickstart).
- [ ] **T011** [P] Test fixture: a minimal agent artifact that connects with its
  injected creds and posts one `turn.post` to its topic.
- [ ] **T010** [US1] Integration test — in-process **operator-mode** NATS +
  provisioned soulstream realm; launch the T011 agent; assert its turn appears
  attributed to the declared persona (**SC-001**), verifiable on the op-log.

## Phase 4: US2 — See a workload's life as ops (P2)

- [ ] **T012** [US2] `runner` abnormal exit — non-zero/signal → `work.abandon`
  (reason), reap scratch + creds even on crash (**SC-004**, FR-010).
- [ ] **T013** [US2] Integration test — follow the topic, assert
  `work.open→claim→done|abandon` in order (**FR-006**); audit a `>` wildcard and
  assert **zero** soulrealm-private control traffic (**SC-002**).

## Phase 5: US3 — Backend-agnostic declaration (P3)

- [ ] **T014** [US3] Explicit **SC-005** assertion (a declaration with a
  `backend` key is rejected) at both the unit and CLI level; a short note in
  `backend/` README documenting the seam the M1.3 Docker backend will implement
  against the unchanged declaration.

## Phase 6: Polish & gate

- [ ] **T015** [P] Run the [`quickstart.md`](quickstart.md) end-to-end against a
  real `nsc` operator realm; fix any drift; update quickstart with the real
  commands observed.
- [ ] **T016** Constitution VI gate: `make fmt && make test && make lint` green,
  nothing skipped. Then land: journey episode (feature done), roadmap M1.1
  closed, design 0001 propagation confirmed.

## Dependencies & parallelism

- T001 → T002 → {T003, T004, T005 in parallel}.
- US1 (T006–T011) needs Phase 2. T006/T007/T011 are parallel; T008 needs
  T006+T007; T010 needs T008+T011.
- US2 (T012–T013) needs T008. US3 (T014) needs T003.
- **MVP = Phase 1 + 2 + US1.** US2 and US3 are independent increments on top.

## Not in this feature (deferred, per plan/research)

Multi-node placement, Docker/Firecracker backends (M1.3), xkey-encrypted
delivery (multi-node), a `work.progress` op (soulstream design, when needed),
object-store attachment scope (first result-producing workload), the `tool`
role (M1.2).

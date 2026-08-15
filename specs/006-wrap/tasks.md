# Tasks: Wrap — run your agent where you are

## Format: `[ID] [P?] Description`

- [ ] T001 `git mv waker wrap`; package rename; delete the daemon half
  (Serve/consumers, dialer lanes, EphemeralMinter/mint, RealmOps waker
  voice); keep template/harness/correlate.
- [ ] T002 `wrap/config.go`: single-agent Config; Template grows `env`;
  presets claude/codex (MCP env derived from lane env); template-file
  load with the specs/005 refusal rules.
- [ ] T003 `wrap/wake.go`: the one-persona protocol — guards,
  outcome-existence, retry budget, discharge (reply | correlated |
  self-report tapping the asker only), WithoutCancel.
- [ ] T004 `wrap/wrap.go`: Wrapper.Run — connect-once (the credential is
  the probe), FetchInbox catch-up, raw notify subscription, reconnect
  re-catch-up, sequential wakes.
- [ ] T005 [P] Unit tests reshaped: call sequences, presets, env
  application, config refusals; correlate/harness suites carried.
- [ ] T006 `cmd/soulstream-wrap`: env contract + flags; loud refusal exit
  on connect failure. `cmd/soulstream-workloads`: remove `waker serve`.
- [ ] T007 Integration: `wrap_test.go` — backlog+live+restart-no-dup
  (SC-001), faults as self-reports (SC-002), operator-server refusal
  (SC-003), codex preset (SC-004); `wrap_live_test.go` + Makefile
  `test-wrap` (SC-005).
- [ ] T008 Gate green (hermetic); `go mod tidy` (drops identity dep);
  merge; hq lands episode/roadmap/WTS + shell fold separately.

# Tasks: The template grows `mcp_args`

## Format: `[ID] [P?] Description`

- [x] T001 `wrap/config.go`: `Template.MCPArgs []string`
  (`mcp_args,omitempty`), `Lane.MCPArgs`, claude preset passthrough;
  doc comments say why args exist (a subcommand door).
- [x] T002 `wrap/harness.go`: `writeMCPConfig` emits `"args"` when
  non-empty; absent otherwise (shape-compatible).
- [x] T003 [P] Tests: preset passthrough + strict-decode acceptance in
  `config_test.go`; written-config arms in `harness_test.go`.
- [x] T004 Gate green (`make check`); merge; hq episode/roadmap ride
  the arc's landing.

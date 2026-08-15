# Feature Specification: The template grows `mcp_args`

**Feature Branch**: `007-mcp-args`
**Created**: 2026-08-15
**Status**: Draft
**Input**: The operator's direction (soul-hq design
[`0004-wrap.md`](../../../soul-hq/02-DESIGN/soulstream-workloads/0004-wrap.md)
§5 as amended, realizing
[`soulstream/0002-wrap-in-the-house.md`](../../../soul-hq/02-DESIGN/soulstream/0002-wrap-in-the-house.md)):
an agent's machine needs no PATH assembly — the tool door must be
launchable as a *subcommand* of the caller's own executable
(`soulstream mcp`), which the per-run MCP config can only express with
an `args` array beside `command`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A subcommand is the tool door (Priority: P1)

The product binary wraps an assistant and points the harness's MCP
config at itself: `command` is its own executable, `args` is `["mcp"]`.
The harness launches the door exactly as any MCP client reads that
config; no second binary exists on the machine.

**Independent Test**: build a `RunSpec` whose template carries
`MCPArgs`; assert the written `mcp.json` carries the `args` array; a
template without `MCPArgs` writes a config with no `args` key
(byte-compatible with today's shape).

**Acceptance Scenarios**:

1. **Given** a template with `mcp_command` and `mcp_args`, **When** a
   wake runs, **Then** the per-run `mcp.json` carries both, 0600.
2. **Given** a template with no `mcp_args`, **When** a wake runs,
   **Then** the written config is shaped exactly as before (no `args`
   key).
3. **Given** a lane carrying `MCPArgs`, **When** the claude preset is
   derived, **Then** the template carries them; the codex preset (which
   owns no tool-door block) is unchanged.
4. **Given** a template file spelling `mcp_args`, **When** it is
   loaded, **Then** it parses under the strict decoder; unknown fields
   still refuse.

## Success Criteria

- **SC-001**: the four scenarios above, hermetic, in `make test`.
- **SC-002**: additive only — every template and preset that exists
  today round-trips unchanged.

# Implementation Plan: The template grows `mcp_args`

**Branch**: `007-mcp-args` | **Date**: 2026-08-15 | **Spec**: [spec.md](spec.md)

## Summary

One additive field through three touch points: `Template.MCPArgs`
(json `mcp_args,omitempty`), `Lane.MCPArgs` (so a caller can point the
door at its own executable), the claude preset passthrough, and
`writeMCPConfig` emitting `"args"` when non-empty.

## Technical Context

**Dependencies**: none added. **Storage**: none. **Testing**: hermetic,
beside the existing suites in `wrap/config_test.go` and
`wrap/harness_test.go`.

## Constitution Check

- **workloads I–III**: PASS by abstention — no substrate, identity, or
  contract change; the template stays the harness axis's whole surface.
- **workloads IV (research gates)**: PASS — design 0004 §5 as amended
  is the basis; no new capability, a shape the MCP config format
  already defines.
- **S2 (smallest viable)**: PASS — one field, one emit site, no new
  seam; the consumer (the product's native wrap) is designed and named.
- **S5**: hermetic default gate unchanged.

## Project Structure

```text
wrap/config.go       # Template.MCPArgs, Lane.MCPArgs, preset passthrough
wrap/harness.go      # writeMCPConfig: "args" when non-empty
wrap/config_test.go  # preset carries lane args; template file parses
wrap/harness_test.go # written config with and without args
```

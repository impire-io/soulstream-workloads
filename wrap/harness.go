package wrap

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// HarnessResult is what one harness run concluded, read from its typed event
// stream — never from its exit code, never from prose.
type HarnessResult struct {
	Text   string // the terminal event's text (valid when OK)
	OK     bool
	Detail string // legible reason on failure, elapsed time on success
}

// RunSpec is one harness invocation, fully resolved: the template, the filled
// prompt, the run directory, and the time budget.
type RunSpec struct {
	Template Template
	Prompt   string
	Topic    string // the wake's topic path, for harnesses that take it as an argument
	RunDir   string
	Timeout  time.Duration
}

// RunHarness executes one wake's harness run: fresh directory, generated MCP
// config, sanitized environment, process-group kill at the deadline, and
// terminal-event extraction by the template's dot-path mapping.
func RunHarness(ctx context.Context, spec RunSpec) HarnessResult {
	if err := os.MkdirAll(spec.RunDir, 0o755); err != nil {
		return HarnessResult{Detail: "run dir: " + err.Error()}
	}
	vars := map[string]string{
		"PROMPT":     spec.Prompt,
		"TOPIC":      spec.Topic,
		"RUN_DIR":    spec.RunDir,
		"MCP_CONFIG": filepath.Join(spec.RunDir, "mcp.json"),
	}
	if spec.Template.MCPCommand != "" {
		if err := writeMCPConfig(spec, vars["MCP_CONFIG"]); err != nil {
			return HarnessResult{Detail: err.Error()}
		}
	}
	argv := make([]string, len(spec.Template.Command))
	for i, a := range spec.Template.Command {
		argv[i] = fill(a, vars)
	}

	runCtx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Dir = spec.RunDir
	// The child sees the host environment scrubbed of SOULSTREAM_*, with the
	// template's own env applied on top — the lane for a per-agent provider
	// credential (ANTHROPIC_API_KEY, …) when the host's login state isn't it.
	env := sanitizedEnv(os.Environ())
	for k, v := range spec.Template.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

	events, err := os.Create(filepath.Join(spec.RunDir, "events.jsonl"))
	if err != nil {
		return HarnessResult{Detail: "events file: " + err.Error()}
	}
	defer func() { _ = events.Close() }()
	stderr, err := os.Create(filepath.Join(spec.RunDir, "stderr.txt"))
	if err != nil {
		return HarnessResult{Detail: "stderr file: " + err.Error()}
	}
	defer func() { _ = stderr.Close() }()
	cmd.Stdout, cmd.Stderr = events, stderr

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return HarnessResult{Detail: "start: " + err.Error()}
	}
	waitErr := cmd.Wait()
	elapsed := time.Since(start).Round(time.Millisecond)

	res := extractTerminal(spec.Template.Terminal, filepath.Join(spec.RunDir, "events.jsonl"))
	if !res.OK && res.Detail == "no terminal event" {
		// The stream never concluded — say why in transport terms.
		switch {
		case runCtx.Err() == context.DeadlineExceeded:
			res.Detail = fmt.Sprintf("run timeout after %s", elapsed)
		case waitErr != nil:
			res.Detail = fmt.Sprintf("harness died (%v) after %s", waitErr, elapsed)
		}
		return res
	}
	if res.OK {
		res.Detail = elapsed.String()
	}
	return res
}

// writeMCPConfig renders the per-run MCP client configuration from the
// template, mode 0600 — it may carry a credential.
func writeMCPConfig(spec RunSpec, path string) error {
	doc := map[string]any{"mcpServers": map[string]any{"soulstream": map[string]any{
		"command": spec.Template.MCPCommand,
		"env":     spec.Template.MCPEnv,
	}}}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("wrap: render mcp config: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("wrap: write mcp config: %w", err)
	}
	return nil
}

// extractTerminal scans the JSONL event stream for the last event matching
// the template's terminal mapping and classifies it.
func extractTerminal(m TerminalMap, eventsPath string) HarnessResult {
	f, err := os.Open(eventsPath)
	if err != nil {
		return HarnessResult{Detail: "events: " + err.Error()}
	}
	defer func() { _ = f.Close() }()

	var terminal map[string]any
	dec := json.NewDecoder(f)
	for {
		var e map[string]any
		if err := dec.Decode(&e); err != nil {
			break // end of stream or trailing garbage — the events so far decide
		}
		if v, ok := dotGet(e, m.TypeField); ok && v == m.TerminalValue {
			terminal = e
		}
	}
	if terminal == nil {
		return HarnessResult{Detail: "no terminal event"}
	}
	text := ""
	if v, ok := dotGet(terminal, m.TextField); ok {
		if s, ok := v.(string); ok {
			text = s
		}
	}
	if m.StatusField != "" {
		if v, ok := dotGet(terminal, m.StatusField); ok {
			if s, _ := v.(string); s != m.SuccessValue {
				return HarnessResult{Detail: fmt.Sprintf("terminal status %q: %s", v, text)}
			}
		}
	}
	if strings.TrimSpace(text) == "" {
		return HarnessResult{Detail: "terminal event carried no text"}
	}
	return HarnessResult{Text: text, OK: true}
}

// dotGet walks a dot-path ("msg.type") through nested JSON objects.
func dotGet(m map[string]any, path string) (any, bool) {
	cur := any(m)
	for _, part := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// fill substitutes {{NAME}} placeholders.
func fill(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	return s
}

// sanitizedEnv strips every SOULSTREAM_* variable: nothing from the host
// environment — least of all the person's own realm configuration — may
// leak into a harness child. The template is the child's whole
// realm-facing surface.
func sanitizedEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		if strings.HasPrefix(kv, "SOULSTREAM_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

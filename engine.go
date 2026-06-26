package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Engine wraps a long-lived `claude` subprocess running in bidirectional
// stream-json mode. It is the *only* place that talks to Claude Code, which
// keeps the auth story simple: because we never set ANTHROPIC_API_KEY (we
// actively strip it), claude uses whatever `claude login` established — your
// Max subscription.
type Engine struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	mu     sync.Mutex // serialises writes to stdin
}

// EngineConfig holds the few knobs worth exposing. Wire your internal tools in
// via MCPConfigPath (a standard .mcp.json); they show up to the agent as MCP
// tools with no change to anything below.
type EngineConfig struct {
	Model          string // "" lets claude pick the account default
	PermissionMode string // "default" | "acceptEdits" | "plan" | "bypassPermissions"
	MCPConfigPath  string // path to a .mcp.json, or "" for none
	ExtraArgs      []string

	// Approvals wiring (set when the in-process permission server is running).
	ApprovalsMCPConfig   string // inline --mcp-config JSON for the approvals server
	PermissionPromptTool string // e.g. mcp__approvals__approve
}

// scrubbedEnv returns the current environment with any credential that would
// override the subscription removed. claude treats both of these as bearer
// tokens, so either one present means you'd silently bill the API instead of
// your plan.
func scrubbedEnv() []string {
	const drop = "ANTHROPIC_API_KEY=ANTHROPIC_AUTH_TOKEN="
	out := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i+1]
		}
		if strings.Contains(drop, key) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// NewEngine spawns the subprocess and returns it ready to stream.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose", // required to receive the full event stream in print mode
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.PermissionMode != "" {
		args = append(args, "--permission-mode", cfg.PermissionMode)
	}
	if cfg.MCPConfigPath != "" {
		args = append(args, "--mcp-config", cfg.MCPConfigPath)
	}
	if cfg.ApprovalsMCPConfig != "" {
		args = append(args, "--mcp-config", cfg.ApprovalsMCPConfig)
	}
	if cfg.PermissionPromptTool != "" {
		args = append(args, "--permission-prompt-tool", cfg.PermissionPromptTool)
	}
	args = append(args, cfg.ExtraArgs...)

	debug.Logf("spawn", "claude %s", strings.Join(args, " "))
	cmd := exec.Command("claude", args...)
	cmd.Env = scrubbedEnv()
	cmd.Stderr = os.Stderr // surface auth/launch errors directly

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Engine{cmd: cmd, stdin: stdin, stdout: stdout}, nil
}

// outUser is the NDJSON envelope we write to stdin for each turn. This is the
// under-documented half of the protocol; the shape matches the Agent SDK's
// streaming-input format. If a future claude version rejects it, check the
// Agent SDK streaming docs for the current envelope.
type outUser struct {
	Type    string `json:"type"`
	Message struct {
		Role    string     `json:"role"`
		Content []outBlock `json:"content"`
	} `json:"message"`
}

type outBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Send writes one user turn to the subprocess. Safe to call from the Bubble Tea
// update loop.
func (e *Engine) Send(text string) error {
	var m outUser
	m.Type = "user"
	m.Message.Role = "user"
	m.Message.Content = []outBlock{{Type: "text", Text: text}}

	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.stdin.Write(append(b, '\n')); err != nil {
		return err
	}
	debug.Logf("stdin", "%s", b)
	return nil
}

// Close ends the session. Call it AFTER the Bubble Tea program exits (p.Run
// returned), never from the Update loop: Wait() blocks until claude exits, and
// from inside Update that deadlocks against the Pipe goroutine (Update stops
// draining p.Send, Pipe stops draining stdout, claude blocks writing and never
// exits). Post-Run, p.Send is a no-op so Pipe keeps draining and an idle claude
// exits promptly on stdin EOF; a busy one is killed after a short grace.
func (e *Engine) Close() {
	if e.stdin != nil {
		_ = e.stdin.Close() // EOF asks an idle claude to exit cleanly
	}
	if e.cmd == nil || e.cmd.Process == nil {
		return
	}
	done := make(chan struct{})
	go func() { _ = e.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = e.cmd.Process.Kill()
		<-done // reap so we don't leave a zombie
	}
}

// ---- streaming into Bubble Tea ----

// streamMsg carries one parsed event into the UI's Update loop.
type streamMsg struct{ env Envelope }

// streamClosedMsg signals stdout hit EOF (subprocess exited).
type streamClosedMsg struct{ err error }

// Pipe reads the subprocess stdout line-by-line and forwards each parsed event
// to the program via p.Send. Run it in its own goroutine after the program is
// constructed. Using p.Send (rather than a tea.Cmd that blocks on a channel)
// keeps backpressure simple and lets the UI stay responsive.
func (e *Engine) Pipe(p *tea.Program) {
	sc := bufio.NewScanner(e.stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // tool results can be large
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		debug.Logf("stdout", "%s", line)
		env, err := parseEnvelope(line)
		if err != nil {
			continue // skip anything that isn't a JSON event line
		}
		p.Send(streamMsg{env: env})
	}
	p.Send(streamClosedMsg{err: sc.Err()})
}

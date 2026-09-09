// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// codexEngine drives a long-lived `codex app-server` subprocess over
// stdio JSON-RPC. It is the codex half of the Engine seam (backend.go).
//
// Two differences from claudeEngine shape everything here.
//
// A turn needs a thread first. claude opens a session implicitly on the first
// turn; codex requires thread/start (or thread/resume) and hands back an id
// that every later call carries. Initialize does that, so a Send always has one.
//
// Mode and model are per-turn, not session-level. claude takes control requests
// mid-session (set_permission_mode, set_model); codex has no equivalent request
// — turn/start takes approvalPolicy, sandbox and model as parameters instead. So
// the setters here record the value and the next turn applies it, which is why
// they cannot fail and never touch the wire.
type codexEngine struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	wmu     sync.Mutex // serialises writes to stdin
	pending *codexPending

	// approvalSlot admits one approval to the pane at a time.
	//
	// The UI holds exactly one pending approval (update.go assigns m.pending),
	// so a second arriving before the first is answered would overwrite it and
	// the first request would never be replied to — and codex waits on a reply
	// forever. claude cannot hit this: its approvals are pulled one at a time by
	// waitApproval. codex pushes, so the serialising has to happen here.
	approvalSlot chan struct{}

	mu       sync.Mutex // guards everything below
	cwd      string     // working root the thread runs in
	resumeID string     // thread to resume on Initialize, or ""
	threadID string
	turnID   string // live turn, learned from turn/started; interrupt needs it
	mode     string // cathode mode, applied at the next turn/start
	model    string
	// sink is where anything bound for the UI goes. Pipe sets it; until then
	// messages are held in backlog. It takes a tea.Msg rather than a codexFrame
	// because an approval request carries a reply channel, which cannot be
	// expressed as JSON — and reusing pendingApprovalMsg is what lets codex
	// share the whole approval pane rather than growing a second one.
	sink    func(tea.Msg)
	backlog []tea.Msg // messages that arrived before Pipe registered a sink
}

// codexEngineConfig is what main resolved for a codex session.
type codexEngineConfig struct {
	Model    string // "" lets codex pick
	Mode     string // cathode mode: ask | plan | build | bypass
	Cwd      string
	ResumeID string // thread to resume, or "" for a fresh one
}

// codexCommand is the binary spawned for a codex session. A package var, not a
// literal, so a test can point it at a scripted stand-in and exercise the real
// reader loop, id correlation and handshake parsing without a network call or a
// billed turn. Same stubbing convention as loadRepoFiles.
var codexCommand = "codex"

// newCodexEngine spawns the app-server and starts reading it.
//
// The reader starts here rather than in Pipe, because Initialize has to
// complete a request/response round trip before the Bubble Tea program exists.
// Frames that arrive in that window are held in backlog and flushed when Pipe
// registers, so nothing from the opening handshake is lost.
func newCodexEngine(cfg codexEngineConfig) (*codexEngine, error) {
	debug.Logf("spawn", "%s app-server", codexCommand)

	cmd := exec.Command(codexCommand, "app-server")
	cmd.Env = codexEnv()
	cmd.Stderr = os.Stderr // surface auth and spawn failures directly

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
	e := &codexEngine{
		cmd: cmd, stdin: stdin, stdout: stdout,
		pending:      newCodexPending(),
		approvalSlot: make(chan struct{}, 1),
		mode:         cfg.Mode,
		model:        cfg.Model,
		cwd:          cfg.Cwd,
		resumeID:     cfg.ResumeID,
	}
	go e.read()
	return e, nil
}

// Pipe registers the program and flushes anything the handshake produced.
// Unlike claudeEngine.Pipe this does not block: reading started at construction.
func (e *codexEngine) Pipe(p *tea.Program) {
	e.mu.Lock()
	e.sink = p.Send
	held := e.backlog
	e.backlog = nil
	e.mu.Unlock()
	for _, msg := range held {
		p.Send(msg)
	}
}

// Close ends the session. Same ordering rule as claudeEngine.Close: main calls
// it after the Bubble Tea program returns, never from the Update loop.
func (e *codexEngine) Close() {
	if e.stdin != nil {
		_ = e.stdin.Close()
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
		<-done // reap, so no zombie is left behind
	}
}

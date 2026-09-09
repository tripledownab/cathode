// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ---- switching backend without restarting (`/backend`) ----
//
// Unlike /sysprompt, this does not re-exec. A backend switch replaces the
// subprocess anyway, and `restartResuming` exists for launch flags that cannot
// change any other way — using it here would throw away the scrollback to
// achieve something the running process can do itself.
//
// What does NOT survive is the conversation. A claude session id means nothing
// to codex, and there is no shared transcript format to hand over, so the new
// backend starts a fresh session. That is stated on screen rather than glossed:
// a switch that silently forgot the context would be worse than one that says
// so.

// backendItems is the /backend picker.
func backendItems(current string) []pickerItem {
	rows := []struct{ id, desc string }{
		{backendClaude, "Claude Code over stream-json"},
		{backendCodex, "codex over app-server JSON-RPC"},
	}
	items := make([]pickerItem, 0, len(rows))
	for _, r := range rows {
		desc := r.desc
		if r.id == sessionBackend(current) {
			desc = "current · " + desc
		}
		items = append(items, pickerItem{id: r.id, title: r.id, subtitle: desc})
	}
	return items
}

// commitBackend swaps the running subprocess for the other backend's.
//
// Every path that declines says why. A silent return reads as "switched" and is
// not — the same rule commitSysPrompt follows.
func (m *model) commitBackend(name string) tea.Cmd {
	name = sessionBackend(name)
	switch {
	case name != backendClaude && name != backendCodex:
		m.add(entError, "unknown backend "+name)
		return nil
	case name == sessionBackend(m.backend):
		m.add(entInfo, "→ already on "+agentName(name))
		return nil
	case m.busy:
		// Mid-turn the old engine still owns a running turn and an approval may
		// be waiting on a reply channel that is about to be dropped.
		m.add(entInfo, "→ backend: busy — interrupt the turn first (esc)")
		return nil
	case m.prog.get() == nil:
		// Nothing to pipe a new engine into. Only reachable before the program
		// starts, but a nil dereference here would take the session with it.
		m.add(entError, "cannot switch backend before the UI is running")
		return nil
	}

	eng, approvals, err := m.spawnBackend(name)
	if err != nil {
		// The old engine is untouched and still running, so the session goes on.
		m.add(entError, "could not start "+agentName(name)+": "+err.Error())
		return nil
	}

	old := m.engine
	m.engine = eng
	m.backend = name
	m.approvals = approvals
	m.resetForBackend()

	// Close off the update loop: Close blocks on Wait, and the whole reason
	// main tears down after p.Run is that doing it inline deadlocks.
	go old.Close()
	go eng.Pipe(m.prog.get())

	m.input.Placeholder = promptPlaceholder(name)
	m.add(entInfo, fmt.Sprintf("— switched to %s · new session, the conversation does not carry over —", agentName(name)))
	m.rerender() // the reply label and header name the backend (agentname.go)
	return tea.Batch(requestModels(eng), waitApproval(approvals))
}

// spawnBackend starts the engine for name, and the approvals server it needs.
//
// claude gates tools through the in-process MCP permission server, so a switch
// to claude has to start one when the session launched on codex without it.
// codex needs none: it raises approvals as requests on its own connection.
func (m *model) spawnBackend(name string) (Engine, *Approvals, error) {
	// A server started for claude is kept when switching to codex, and reused
	// on the way back. codex never routes through it, and rebinding a port on
	// every switch would be work for nothing.
	approvals := m.approvals
	cfg := m.engineCfg
	cfg.PermissionMode = modeToPermission(m.mode)

	if name == backendClaude && approvals == nil && m.mode != "bypass" {
		a, err := StartApprovals()
		if err != nil {
			// The pane is worth less than the session: report it and run
			// ungated, the same trade main makes at launch.
			m.add(entError, "approvals disabled: "+err.Error())
		} else {
			approvals = a
		}
	}
	if approvals != nil {
		cfg.ApprovalsMCPConfig = approvals.mcpConfigJSON()
		cfg.PermissionPromptTool = approvals.permissionToolName()
	}

	// No resume, and no model. The other backend has never seen this session's
	// id, and a model id resolved by one backend names nothing in the other's
	// catalogue. The launch --resume has to be stripped from ExtraArgs too, or
	// switching a resumed session hands claude an id from codex.
	cfg.Model = ""
	cfg.ExtraArgs = withoutResume(cfg.ExtraArgs)
	eng, err := startEngine(name, cfg, m.mode, "", "")
	if err != nil {
		return nil, nil, err
	}
	return eng, approvals, nil
}

// resetForBackend clears everything that described the old subprocess.
//
// The transcript stays — it is what the user read, and losing it would make a
// switch feel like a restart. Everything else belonged to a session that no
// longer exists, and a stale value here is worse than an empty one: a model
// list from the wrong backend offers rows that cannot be selected, and a
// carried-over session id would be written to the store under the new backend.
func (m *model) resetForBackend() {
	m.session = ""
	m.modelID = ""
	m.agentCwd = ""
	m.models = nil
	m.commands = nil
	m.agents = nil
	m.mcpServers = nil
	m.toolUses = nil
	m.shownTools = nil
	m.lastCost = 0
	m.outTokens = 0
	m.ctxTokens = 0
	m.busy = false
	m.stopCompacting()

	// The context window belongs to the model that is gone. Back to the -ctx
	// floor until the new backend reports or grows past it, rather than keeping
	// a number the new session has no relation to.
	m.ctxLimit = m.baseCtxLimit
}

// withoutResume drops a --resume flag from a launch argv, in both the joined
// and the separated form. Returns a fresh slice: the caller's config is reused
// on the next switch and must not be edited underneath it.
func withoutResume(args []string) []string {
	out := make([]string, 0, len(args))
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if a == "--resume" || a == "-resume" {
			skip = true
			continue
		}
		if strings.HasPrefix(a, "--resume=") || strings.HasPrefix(a, "-resume=") {
			continue
		}
		out = append(out, a)
	}
	return out
}

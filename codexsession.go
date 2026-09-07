// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
)

// ---- the Engine seam ----

// Initialize runs the handshake and opens the thread every later call needs.
//
// Three steps, in order, because codex requires all of them before a turn:
// the initialize request, the initialized notification that completes it, and
// thread/start (or thread/resume). The thread id lives at result.thread.id —
// not result.threadId, which is the shape a reader would assume.
func (e *codexEngine) Initialize() error {
	if _, err := e.call("initialize", map[string]any{
		"clientInfo":   map[string]string{"name": clientName, "version": clientVersion},
		"capabilities": map[string]any{"experimentalApi": true},
	}); err != nil {
		return err
	}
	if err := e.notify("initialized", map[string]any{}); err != nil {
		return err
	}
	return e.openThread()
}

// openThread starts a fresh thread, or resumes one when main was given an id.
func (e *codexEngine) openThread() error {
	e.mu.Lock()
	resume, mode, cwd := e.resumeID, e.mode, e.cwd
	e.mu.Unlock()

	policy, sandbox := codexPolicyForMode(mode)
	params := map[string]any{
		"approvalPolicy": policy,
		"sandbox":        sandbox,
	}
	// State the working root rather than relying on the inherited one. diff.go
	// reads a file's "before" off disk on the assumption that the agent runs in
	// the TUI's cwd, so the two must not be free to differ.
	if cwd != "" {
		params["cwd"] = cwd
	}
	method := "thread/start"
	if resume != "" {
		method, params["threadId"] = "thread/resume", resume
	}
	res, err := e.call(method, params)
	if err != nil {
		return err
	}
	var out struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return err
	}
	if out.Thread.ID == "" {
		return errors.New("codex: " + method + " returned no thread id")
	}
	e.mu.Lock()
	e.threadID = out.Thread.ID
	e.mu.Unlock()
	return nil
}

// Send opens one turn. Non-blocking: turn/start replies as soon as the turn is
// accepted, long before it finishes, and the reply carries the turn id that
// Interrupt needs.
func (e *codexEngine) Send(text string) error {
	e.mu.Lock()
	thread, mode, model := e.threadID, e.mode, e.model
	e.mu.Unlock()
	if thread == "" {
		return errors.New("codex: no thread yet — Initialize first")
	}

	policy, sandbox := codexPolicyForMode(mode)
	params := map[string]any{
		"threadId":       thread,
		"input":          []map[string]any{{"type": "text", "text": text}},
		"approvalPolicy": policy,
		"sandboxPolicy":  codexSandboxPolicy(sandbox),
	}
	if model != "" {
		params["model"] = model
	}
	return e.fire("turn/start", params, func(res json.RawMessage) {
		var out struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(res, &out) == nil && out.Turn.ID != "" {
			e.mu.Lock()
			e.turnID = out.Turn.ID
			e.mu.Unlock()
		}
	})
}

// Interrupt aborts the running turn. codex needs the turn id as well as the
// thread id, so there is nothing to do when no turn is in flight.
func (e *codexEngine) Interrupt() error {
	e.mu.Lock()
	thread, turn := e.threadID, e.turnID
	e.mu.Unlock()
	if thread == "" || turn == "" {
		return nil // no turn running; the UI clears busy either way
	}
	return e.fire("turn/interrupt", map[string]any{"threadId": thread, "turnId": turn}, nil)
}

// SetPermissionMode records the mode for the next turn.
//
// It sends nothing. codex has no mid-session equivalent of claude's
// set_permission_mode control request — approvalPolicy and sandbox are
// parameters of turn/start — so the change lands when the next turn opens.
// Shift+Tab therefore takes effect on the next turn, not the running one.
func (e *codexEngine) SetPermissionMode(mode string) error {
	e.mu.Lock()
	e.mode = mode
	e.mu.Unlock()
	return nil
}

// SetModel records the model for the next turn, for the same reason.
func (e *codexEngine) SetModel(model string) error {
	e.mu.Lock()
	e.model = model
	e.mu.Unlock()
	return nil
}

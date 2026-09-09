// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// The stdout side of the codex connection: one goroutine consuming frames and
// deciding where each one goes. Kept apart from the engine's lifecycle so the
// routing rule stays readable on its own.

// read is the single consumer of stdout. It sorts each frame and never blocks
// on the UI: a reply goes to its waiter, everything else goes to the program.
func (e *codexEngine) read() {
	sc := bufio.NewScanner(e.stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // tool output can be large
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		debug.Logf("stdout", "%s", line)
		var f codexFrame
		if err := json.Unmarshal(line, &f); err != nil {
			continue // not a frame; skip rather than kill the session
		}
		switch f.classify() {
		case frameResponse:
			e.pending.deliver(f)
			continue
		case frameServerReq:
			// Answered here, not in the UI: an unanswered request stops the
			// turn dead with no error shown anywhere (codexapproval.go).
			e.answerServerRequest(f)
			continue
		}
		e.noteTurn(f)
		e.emit(f)
	}
	// The subprocess is gone. Wake every in-flight call now: left alone each
	// one waits out its own timeout and the UI sits frozen meanwhile.
	e.pending.failAll(errEngineClosed)
	e.emit(codexFrame{Method: codexClosedMethod})
}

// noteTurn keeps the live turn id current. turn/interrupt needs it alongside
// the thread id, and the notification stream is the only place it appears.
func (e *codexEngine) noteTurn(f codexFrame) {
	switch f.Method {
	case "turn/started":
		var p struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(f.Params, &p) == nil {
			e.mu.Lock()
			e.turnID = p.Turn.ID
			e.mu.Unlock()
		}
	case "turn/completed", "turn/failed":
		e.mu.Lock()
		e.turnID = ""
		e.mu.Unlock()
	}
}

// emit forwards one frame to the sink, or holds it until one is registered.
// The send happens outside the lock: a slow consumer must not block the reader,
// which would stall every later frame behind it.
func (e *codexEngine) emit(f codexFrame) { e.emitMsg(codexMsg{frame: f}) }

// emitMsg forwards any message to the sink, or holds it until one is
// registered. The send happens outside the lock: a slow consumer must not block
// the reader, which would stall every later message behind it.
func (e *codexEngine) emitMsg(msg tea.Msg) {
	e.mu.Lock()
	sink := e.sink
	if sink == nil {
		e.backlog = append(e.backlog, msg)
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()
	sink(msg)
}

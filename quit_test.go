package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// isQuit runs a Cmd and reports whether it yields tea.QuitMsg.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// The quit paths must return tea.Quit WITHOUT closing the engine — Close()
// blocks (Wait), and from the Update loop that deadlocks against the Pipe
// goroutine (the "one ctrl+c freezes" bug). The subprocess is torn down in main
// after Run() instead. A nil engine here would panic if Close were still called.
func TestQuitPathsDoNotTouchEngine(t *testing.T) {
	// ctrl+c with nothing pending
	if _, cmd, handled := (model{}).handleKey(tea.KeyMsg{Type: tea.KeyCtrlC}); !handled || !isQuit(cmd) {
		t.Error("ctrl+c should quit")
	}

	// esc while idle (no queue, not busy)
	if _, cmd, handled := (model{}).handleEsc(); !handled || !isQuit(cmd) {
		t.Error("esc while idle should quit")
	}

	// /quit
	if _, cmd, _ := runSlash(&model{}, "/quit"); !isQuit(cmd) {
		t.Error("/quit should quit")
	}

	// ctrl+c during a pending approval: denies (buffered reply) then quits
	pending := &approvalReq{toolName: "Edit", reply: make(chan bool, 1)}
	m := model{pending: pending}
	_, cmd, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !handled || !isQuit(cmd) {
		t.Error("ctrl+c during approval should quit")
	}
	if got := <-pending.reply; got {
		t.Error("ctrl+c during approval should deny the pending request")
	}
}

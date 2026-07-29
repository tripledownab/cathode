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
	// esc while idle (not busy) quits
	if _, cmd, handled := (model{}).handleEsc(); !handled || !isQuit(cmd) {
		t.Error("esc while idle should quit")
	}

	// /quit
	if _, cmd, _ := runSlash(&model{}, "/quit"); !isQuit(cmd) {
		t.Error("/quit should quit")
	}

	// A second ctrl+c (the model returned by the first, now armed) quits.
	armed, _, _ := (model{}).handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if _, cmd, handled := armed.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC}); !handled || !isQuit(cmd) {
		t.Error("second ctrl+c should quit")
	}
}

// Ctrl+C no longer exits on the first press: idle it arms the "again to exit"
// window, and during a turn it interrupts. Only a second press within the
// window quits. (Don't run the returned Cmd — it's the hint tick, which blocks
// ctrlCExitWindow; assert on the armed state instead.)
func TestCtrlCArmsBeforeQuitting(t *testing.T) {
	m, _, handled := (model{}).handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !handled {
		t.Fatal("ctrl+c should be handled")
	}
	if !m.ctrlCArmed() {
		t.Error("first ctrl+c should arm the exit window, not quit")
	}
}

// Ctrl+C during a pending approval denies the tool (cancels the action) and
// arms the exit window, but does not quit on the first press.
func TestCtrlCDuringApprovalDeniesAndArms(t *testing.T) {
	pending := &approvalReq{toolName: "Edit", reply: make(chan approvalReply, 1)}
	m := model{pending: pending}
	m, _, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !handled {
		t.Fatal("ctrl+c during approval should be handled")
	}
	if (<-pending.reply).allow {
		t.Error("ctrl+c during approval should deny the pending request")
	}
	if m.pending != nil {
		t.Error("ctrl+c should clear the pending approval")
	}
	if !m.ctrlCArmed() {
		t.Error("ctrl+c during approval should arm the exit window")
	}
}

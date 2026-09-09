// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeEngine records what the UI asked the subprocess to do. It exists so the
// send path can be tested without spawning anything: before the Engine seam
// there was no way to assert what actually reached the wire, only what the
// helpers returned in isolation.
type fakeEngine struct {
	sent     []string
	shutdown bool
}

func (f *fakeEngine) Send(text string) error { f.sent = append(f.sent, text); return nil }

// Close is recorded because a declined backend switch must leave the old engine
// running, and that is only checkable by asking whether it was shut down
// (backendswitch_test.go). The rest stay unrecorded: a field nobody asserts is
// a field that can drift from what it claims to capture. Record one when a test
// needs it.
func (f *fakeEngine) Close()                              { f.shutdown = true }
func (f *fakeEngine) Initialize() error                   { return nil }
func (f *fakeEngine) Interrupt() error                    { return nil }
func (f *fakeEngine) SetPermissionMode(mode string) error { return nil }
func (f *fakeEngine) SetModel(m string) error             { return nil }
func (f *fakeEngine) Pipe(*tea.Program)                   {}

var _ Engine = (*fakeEngine)(nil)

// last returns the most recent send, or "" when nothing was sent.
func (f *fakeEngine) last() string {
	if len(f.sent) == 0 {
		return ""
	}
	return f.sent[len(f.sent)-1]
}

// newTestModel builds a model around a fake engine, with state redirected to a
// temp dir so a test never appends to the real prompt history.
func newTestModel(t *testing.T, sysPrompt string) (model, *fakeEngine) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	f := &fakeEngine{}
	return newModel(launchConfig{Engine: f, Mode: "ask", Spinner: "bar", SysPrompt: sysPrompt}), f
}

// The transcript shows what the user typed; the wire carries the reminder too.
// Nothing pinned that pairing before — remind.go's helpers were tested in
// isolation, so dropping the withReminder call from sendTurn stayed green.
func TestSendTurnAppliesTheReminderToTheWireOnly(t *testing.T) {
	m, f := newTestModel(t, "Be terse.")

	m.sendTurn("explain the engine")

	if got := f.last(); !strings.Contains(got, "<"+reminderTag+">") {
		t.Errorf("the reminder never reached the engine, sent %q", got)
	}
	if got := stripReminder(f.last()); got != "explain the engine" {
		t.Errorf("the user's text did not survive the append, got %q", got)
	}
	if n := len(m.entries); n == 0 {
		t.Fatal("the turn should be in the transcript")
	}
	last := m.entries[len(m.entries)-1]
	if last.kind != entUser || last.text != "explain the engine" {
		t.Errorf("transcript entry = %+v, want the typed text as entUser", last)
	}
	if strings.Contains(last.text, reminderTag) {
		t.Error("the reminder must not be shown in the transcript")
	}
}

// No style loaded means nothing to remind about, so the turn goes out as typed.
func TestSendTurnSendsVerbatimWithoutAStyle(t *testing.T) {
	m, f := newTestModel(t, "")

	m.sendTurn("explain the engine")

	if got := f.last(); got != "explain the engine" {
		t.Errorf("sent %q, want the turn unchanged", got)
	}
}

// A slash command forwarded to claude is an argument list. The palette sends
// "/"+name through this same path (pickerkeys.go), so this is the real route,
// not a hypothetical one.
func TestSendTurnLeavesForwardedSlashCommandsAlone(t *testing.T) {
	m, f := newTestModel(t, "Be terse.")

	m.sendTurn("/compact")

	if got := f.last(); got != "/compact" {
		t.Errorf("sent %q, want the command unchanged", got)
	}
}

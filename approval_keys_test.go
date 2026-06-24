package main

import (
	"encoding/json"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// driveApproval is the shared harness: stand up a model with a pending request,
// inject one key, and return the decision the approval handler made.
func driveApproval(t *testing.T, key tea.KeyMsg) bool {
	t.Helper()
	m := newModel(&Engine{}, "ask", nil, "bar", "")
	// newModel sets splash=true; the splash eats the first keypress, which
	// would defeat the test. Skip past it.
	m.splash = false
	reply := make(chan bool, 1)
	m.pending = &approvalReq{
		toolName: "Edit",
		input:    json.RawMessage(`{}`),
		reply:    reply,
	}
	_, _ = m.Update(key)
	select {
	case v := <-reply:
		return v
	case <-time.After(500 * time.Millisecond):
		t.Fatal("approval handler never replied")
		return false
	}
}

// TestApprovalStrayLetterAllows pins the deny-on-stray-keypress fix: a bare "n"
// arriving because the user was mid-typing must NOT deny the request. Any key
// other than the explicit Esc gesture defaults to ALLOW.
func TestApprovalStrayLetterAllows(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'n'}},
		{Type: tea.KeyRunes, Runes: []rune{'N'}},
		{Type: tea.KeyRunes, Runes: []rune{'x'}},
		{Type: tea.KeyEnter},
		{Type: tea.KeySpace},
	} {
		if !driveApproval(t, key) {
			t.Fatalf("key %q should have defaulted to allow but denied", key.String())
		}
	}
}

// TestApprovalEscDenies confirms the only "deny" gesture is Esc.
func TestApprovalEscDenies(t *testing.T) {
	if driveApproval(t, tea.KeyMsg{Type: tea.KeyEsc}) {
		t.Fatal("Esc should deny but allowed")
	}
}

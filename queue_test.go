package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Editing a queued message must replace it, not duplicate it. Before the fix,
// up-arrow recalled the queued text from history while leaving it on the queue,
// so the original went out as-is and the edit landed as a second queued item.
func TestUpArrowEditsQueuedMessage(t *testing.T) {
	m := inputModel("")
	m.mouse = true // plain up does history/queue recall (not transcript scroll)
	m.hist = &history{}
	m.busy = true
	m.queue = []string{"original message"}

	// Up-arrow pulls the queued message out for editing and empties the queue.
	m, _, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	if !handled {
		t.Fatal("up-arrow with a queued message should be handled")
	}
	if got := m.input.Value(); got != "original message" {
		t.Fatalf("input = %q, want the queued message pulled in for editing", got)
	}
	if len(m.queue) != 0 {
		t.Fatalf("queue = %v, want empty (message moved into the prompt)", m.queue)
	}

	// Editing and re-sending replaces it — the queue holds only the edited text.
	m.input.SetValue("edited message")
	m, _, _ = m.handleEnter()
	if len(m.queue) != 1 || m.queue[0] != "edited message" {
		t.Fatalf("queue = %v, want exactly [edited message] (no duplicate original)", m.queue)
	}
}

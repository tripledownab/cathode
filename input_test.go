package main

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func inputModel(val string) model {
	ta := newPromptArea() // same config as the real app (keymap, prompt, …)
	ta.SetWidth(40)
	ta.SetValue(val)
	ta.Focus()
	return model{input: ta, w: 50, h: 20, ready: true}
}

// A trailing backslash turns Enter into a newline rather than a submission.
func TestBackslashNewline(t *testing.T) {
	m := inputModel("hello\\")
	nm, cmd, handled := m.handleEnter()
	if !handled || cmd != nil {
		t.Fatalf("backslash+enter should insert a newline, not submit (handled=%v cmd!=nil=%v)", handled, cmd != nil)
	}
	if nm.input.Value() != "hello\n" {
		t.Fatalf("value = %q, want %q", nm.input.Value(), "hello\n")
	}
}

// alt+enter (Enter with the Alt modifier) is a newline, so handleKey must NOT
// treat it as a submission — it falls through to the textarea.
func TestAltEnterFallsThrough(t *testing.T) {
	m := inputModel("hi")
	if _, _, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter, Alt: true}); handled {
		t.Error("alt+enter should fall through to the textarea, not submit")
	}
	// plain Enter on an empty prompt is a no-op (not a submission).
	if _, _, handled := inputModel("").handleKey(tea.KeyMsg{Type: tea.KeyEnter}); handled {
		t.Error("enter on an empty prompt should not submit")
	}
}

// In a multi-line draft, plain up moves the cursor (falls through); it doesn't
// trigger history recall.
func TestMultiLineUpFallsThrough(t *testing.T) {
	m := inputModel("line one\nline two")
	if _, _, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyUp}); handled {
		t.Error("up in a multi-line draft should fall through to the textarea")
	}
}

// The prompt grows to fit its line count.
func TestPromptGrowsWithLines(t *testing.T) {
	m := inputModel("a\nb\nc")
	m.syncPromptHeight()
	if got := m.promptRows(); got != 3 {
		t.Errorf("promptRows = %d, want 3", got)
	}
}

// End-to-end through the real Update loop: ctrl+j inserts a newline (textarea
// keybinding) and the prompt grows — exercising handleKey fall-through, the
// textarea's Update, and syncPromptHeight in the tail.
func TestUpdateCtrlJGrowsPrompt(t *testing.T) {
	m := inputModel("hello")
	m.vp = newTranscriptViewport(40, 6)
	m.lastActivity = time.Now()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	nm := next.(model)
	if nm.input.Value() != "hello\n" {
		t.Fatalf("ctrl+j should insert a newline, value=%q", nm.input.Value())
	}
	if got := nm.promptRows(); got != 2 {
		t.Fatalf("prompt should grow to 2 rows, got %d", got)
	}
}

// A pasted multi-line block (bracketed paste) lands as newlines and grows the
// prompt too.
func TestUpdatePasteGrowsPrompt(t *testing.T) {
	m := inputModel("")
	m.vp = newTranscriptViewport(40, 6)
	m.lastActivity = time.Now()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("one\ntwo\nthree"), Paste: true})
	nm := next.(model)
	if nm.promptRows() != 3 {
		t.Fatalf("pasted 3 lines should grow the prompt to 3 rows, got %d (value=%q)", nm.promptRows(), nm.input.Value())
	}
}

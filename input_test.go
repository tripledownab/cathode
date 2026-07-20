package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func inputModel(val string) model {
	m := model{w: 50, h: 20, ready: true}
	m.input = newPromptArea() // same config as the real app (keymap, prompt, …)
	m.setPromptWidth(40)      // inner wrap width 38 (prompt "› " is 2 cells)
	m.input.SetValue(val)
	m.input.Focus()
	return m
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

// A long single line soft-wraps in a narrow window; the prompt must grow to
// show every wrapped row (sizing by hard lines alone left it one row tall,
// scrolled to the cursor — typing blind). Capped at maxPromptRows.
func TestPromptGrowsOnSoftWrap(t *testing.T) {
	m := inputModel(strings.Repeat("word ", 20)) // ~100 cells at inner width 38
	m.syncPromptHeight()
	if got := m.promptRows(); got < 3 {
		t.Errorf("~100 cells at wrap width 38 should need ≥3 rows, got %d", got)
	}

	long := inputModel(strings.Repeat("word ", 200))
	long.syncPromptHeight()
	if got := long.promptRows(); got != maxPromptRows {
		t.Errorf("overflow should cap at %d rows, got %d", maxPromptRows, got)
	}
}

// The reported bug: a newline keystroke moved the cursor to a new row while
// the widget was still its OLD (shorter) height, scrolling its internal
// viewport — and SetHeight doesn't reset that scroll, so the grown prompt
// showed only the last row. Drive the REAL Update loop: type, insert a line
// break (ctrl+j), type more — every row must stay on screen.
func TestNewlineKeepsFirstRowVisible(t *testing.T) {
	cur := inputModel("")
	cur.vp = newTranscriptViewport(40, 6)
	cur.lastActivity = time.Now()
	feed := func(msg tea.KeyMsg) {
		next, _ := cur.Update(msg)
		cur = next.(model)
		// Render between keystrokes like the real program does — the textarea's
		// internal viewport only ingests content during View, and its scrolling
		// (the bug's trigger) acts on that state on the NEXT keystroke.
		_ = cur.input.View()
	}
	for _, r := range "ALPHA line" {
		feed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	feed(tea.KeyMsg{Type: tea.KeyCtrlJ}) // the reported trigger
	for _, r := range "second" {
		feed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if rows := cur.promptRows(); rows != 2 {
		t.Fatalf("prompt should be 2 rows, got %d", rows)
	}
	view := stripANSI(cur.input.View())
	if !strings.Contains(view, "ALPHA") {
		t.Fatalf("first row scrolled out of view after the newline:\n%s", view)
	}
	if !strings.Contains(view, "second") {
		t.Fatalf("second row missing:\n%s", view)
	}
	// The cursor survived the re-anchor at the end of the draft.
	feed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Z'}})
	if v := cur.input.Value(); !strings.HasSuffix(v, "secondZ") {
		t.Fatalf("cursor not preserved at end after reanchor, value=%q", v)
	}
}

// Fidelity: size the widget to our computed row count and confirm nothing is
// clipped. SetValue leaves the cursor at the END, so the textarea scrolls its
// tail into view — if promptVisualRows ever undercounts the real wrapped rows,
// the FIRST token would fall off the top of the widget's view.
func TestSoftWrapCountMatchesWidget(t *testing.T) {
	for _, val := range []string{
		"ALPHA short",
		"ALPHA " + strings.Repeat("lorem ipsum dolor ", 8),
		"ALPHA " + strings.Repeat("x", 90), // one unbroken over-width word
		"ALPHA first\nsecond line that is long enough to wrap at width thirty-eight",
	} {
		m := inputModel(val)
		rows := promptVisualRows(val, m.promptInnerWidth())
		m.input.SetHeight(rows)
		if view := stripANSI(m.input.View()); !strings.Contains(view, "ALPHA") {
			t.Errorf("undercounted rows (%d) for %q — first token scrolled out:\n%s", rows, val, view)
		}
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

// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// withStubFiles swaps the git-backed file source for a fixed list so the
// completion tests don't depend on the working tree. Returns a restore func.
func withStubFiles(files []string) func() {
	prev := loadRepoFiles
	loadRepoFiles = func() []string { return files }
	return func() { loadRepoFiles = prev }
}

func TestAtToken(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		col   int
		query string
		at    int
		ok    bool
	}{
		{"bare at", "@", 1, "", 0, true},
		{"at start with path", "@src/main.go", 12, "src/main.go", 0, true},
		{"after space", "see @rea", 8, "rea", 4, true},
		{"email is not a mention", "foo@bar", 7, "", 0, false},
		{"space closed the token", "@foo bar", 8, "", 0, false},
		{"no at at all", "hello", 5, "", 0, false},
		{"nearest at wins", "@a @b", 5, "b", 3, true},
		{"cursor mid-token", "@abcd", 3, "ab", 0, true},
	}
	for _, tc := range cases {
		q, at, ok := atToken(tc.line, tc.col)
		if ok != tc.ok || q != tc.query || (ok && at != tc.at) {
			t.Errorf("%s: atToken(%q,%d) = (%q,%d,%v), want (%q,%d,%v)",
				tc.name, tc.line, tc.col, q, at, ok, tc.query, tc.at, tc.ok)
		}
	}
}

// Typing @ then a query opens the menu and fuzzy-ranks the file list; a cursor
// past the token (a space intervening) keeps it closed.
func TestCompletionOpensAndRanks(t *testing.T) {
	defer withStubFiles([]string{"go.mod", "main.go", "keys.go"})()

	m := inputModel("see @ma")
	m.syncCompletion()
	if m.comp == nil {
		t.Fatal("menu should open for @ma")
	}
	if sel, _ := m.comp.selected(); sel != "main.go" {
		t.Errorf("top match = %q, want main.go", sel)
	}

	closed := inputModel("see @ma done")
	closed.syncCompletion()
	if closed.comp != nil {
		t.Error("menu should be closed when the cursor is past the token")
	}
}

// Accepting inserts "@path " over the typed @token and closes the menu; the
// leading @ is kept because the claude subprocess expands it to file contents.
func TestAcceptCompletionInsertsAtPath(t *testing.T) {
	defer withStubFiles([]string{"go.mod"})()

	m := inputModel("explain @go")
	m.syncCompletion()
	if m.comp == nil {
		t.Fatal("expected the menu to be open")
	}
	m.acceptCompletion()
	if got := m.input.Value(); got != "explain @go.mod " {
		t.Errorf("value = %q, want %q", got, "explain @go.mod ")
	}
	if m.comp != nil {
		t.Error("menu should close after accept")
	}
}

// Enter routed through the real key dispatcher accepts the completion rather
// than submitting the turn (cmd is nil, no turn sent).
func TestHandleKeyEnterAcceptsCompletion(t *testing.T) {
	defer withStubFiles([]string{"go.mod"})()

	m := inputModel("@go")
	m.syncCompletion()
	nm, cmd, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("enter should be handled by the completion menu")
	}
	if cmd != nil {
		t.Error("accepting a completion must not submit a turn")
	}
	if got := nm.input.Value(); got != "@go.mod " {
		t.Errorf("value = %q, want %q", got, "@go.mod ")
	}
}

// Down through the dispatcher moves the menu cursor (and doesn't fall through to
// history recall).
func TestHandleKeyDownNavigatesCompletion(t *testing.T) {
	defer withStubFiles([]string{"a.go", "ab.go", "abc.go"})()

	m := inputModel("@a")
	m.syncCompletion()
	if m.comp == nil || len(m.comp.filtered) != 3 {
		t.Fatalf("expected 3 matches for @a, got %v", m.comp)
	}
	start := m.comp.cursor
	nm, _, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if !handled {
		t.Fatal("down should be handled by the completion menu")
	}
	if nm.comp.cursor != (start+1)%3 {
		t.Errorf("cursor = %d, want %d", nm.comp.cursor, (start+1)%3)
	}
}

// Esc dismisses the menu and suppresses reopening while the token persists;
// once the token is gone, a fresh @ opens it again.
func TestCompletionEscDismissUntilTokenLeft(t *testing.T) {
	defer withStubFiles([]string{"main.go"})()

	m := inputModel("@ma")
	m.syncCompletion()
	if m.comp == nil {
		t.Fatal("menu should be open")
	}
	nm, _, handled := m.handleCompletionKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !handled || nm.comp != nil || !nm.compDismissed {
		t.Fatalf("esc should close and mark dismissed (comp=%v dismissed=%v)", nm.comp, nm.compDismissed)
	}

	nm.input.SetValue("@main")
	nm.syncCompletion()
	if nm.comp != nil {
		t.Error("menu should stay dismissed while the token persists")
	}

	nm.input.SetValue("hello ")
	nm.syncCompletion()
	if nm.compDismissed {
		t.Error("dismissal should reset once the token is gone")
	}
	nm.input.SetValue("hello @m")
	nm.syncCompletion()
	if nm.comp == nil {
		t.Error("a fresh @ should reopen the menu")
	}
}

// End-to-end through the real Update loop and View: typing "@k" opens the menu,
// which renders the matching file above the prompt without disturbing the frame
// height (the overlay splices in place).
func TestCompletionMenuRendersAbovePrompt(t *testing.T) {
	defer withStubFiles([]string{"go.mod", "main.go", "keys.go"})()

	var tm tea.Model = func() model { m := newModel(&Engine{}, "ask", nil, "bar", ""); m.splash = false; return m }()
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	fm := tm.(model)

	if fm.comp == nil {
		t.Fatal("menu should be open after typing @k")
	}
	view := stripANSI(fm.View())
	if !strings.Contains(view, "keys.go") {
		t.Fatalf("menu should list keys.go:\n%s", view)
	}
	if h := strings.Count(view, "\n") + 1; h != 24 {
		t.Errorf("View height = %d, want 24 (overlay must splice in place)", h)
	}
	lines := strings.Split(view, "\n")
	menuRow, promptRow := -1, -1
	for i, ln := range lines {
		if menuRow < 0 && strings.Contains(ln, "keys.go") {
			menuRow = i
		}
		if strings.Contains(ln, "›") {
			promptRow = i
		}
	}
	if menuRow < 0 || promptRow < 0 || menuRow >= promptRow {
		t.Errorf("menu (row %d) should render above the prompt (row %d)", menuRow, promptRow)
	}
}

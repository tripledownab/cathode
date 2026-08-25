// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// completionRows is how many file rows the @-menu shows before it windows.
const completionRows = 8

// completion is the inline @-mention file picker that floats above the prompt.
// Unlike the modal `picker`, it lives alongside the focused textarea: you keep
// typing and it filters live, with ↑/↓ to move and tab/enter to insert. The
// chosen "@path" is expanded to the file's contents by the claude subprocess
// (verified against the stream-json input path), so this is real mention
// support, not just a path-typing shortcut. items is the candidate list loaded
// once when the menu opens; filtered is the fuzzy-ranked subset for the query.
type completion struct {
	query    string
	items    []string
	filtered []int
	cursor   int
}

func (c *completion) setQuery(q string) {
	c.query = q
	c.refilter()
}

// refilter reranks items for the current query, reusing the picker's fuzzy
// scorer. An empty query keeps the loaded (alphabetical) order.
func (c *completion) refilter() {
	c.filtered = c.filtered[:0]
	if c.query == "" {
		for i := range c.items {
			c.filtered = append(c.filtered, i)
		}
		c.clampCursor()
		return
	}
	type scored struct{ idx, score int }
	ranked := make([]scored, 0, len(c.items))
	for i, it := range c.items {
		if s, ok := fuzzyScore(c.query, it); ok {
			ranked = append(ranked, scored{i, s})
		}
	}
	sort.SliceStable(ranked, func(a, b int) bool { return ranked[a].score > ranked[b].score })
	for _, s := range ranked {
		c.filtered = append(c.filtered, s.idx)
	}
	c.clampCursor()
}

func (c *completion) clampCursor() {
	if c.cursor < 0 || c.cursor >= len(c.filtered) {
		c.cursor = 0
	}
}

// move advances the cursor by d, wrapping around the filtered list.
func (c *completion) move(d int) {
	if len(c.filtered) == 0 {
		return
	}
	c.cursor = (c.cursor + d + len(c.filtered)) % len(c.filtered)
}

func (c *completion) selected() (string, bool) {
	if len(c.filtered) == 0 {
		return "", false
	}
	return c.items[c.filtered[c.cursor]], true
}

// atToken finds an active @-mention under the cursor on a single line. Given the
// line and the cursor's rune column, it scans back to the nearest '@' that
// begins a word (start-of-line or after whitespace) and returns the text between
// it and the cursor. ok is false when the cursor isn't inside such a token — no
// '@', an '@' mid-word (e.g. an email address), or a space that already closed
// the token. at is the rune index of the '@'.
func atToken(line string, col int) (query string, at int, ok bool) {
	runes := []rune(line)
	if col > len(runes) {
		col = len(runes)
	}
	if col < 0 {
		col = 0
	}
	for i := col - 1; i >= 0; i-- {
		r := runes[i]
		if unicode.IsSpace(r) {
			return "", 0, false
		}
		if r == '@' {
			if i == 0 || unicode.IsSpace(runes[i-1]) {
				return string(runes[i+1 : col]), i, true
			}
			return "", 0, false
		}
	}
	return "", 0, false
}

// promptCursor returns the textarea cursor as a (hard-line row, rune column).
// bubbles exposes the row (Line) but not the column, so we recover it from
// LineInfo: StartColumn is where the current soft-wrapped row begins in the hard
// line and CharOffset is the cursor's offset within it. Exact for the ASCII text
// of file paths and ordinary prompts.
func promptCursor(ta textarea.Model) (row, col int) {
	li := ta.LineInfo()
	return ta.Line(), li.StartColumn + li.CharOffset
}

// syncCompletion opens, updates, or closes the @-menu from the current prompt
// text and cursor. Called once per Update after the textarea has handled the
// key, so the token reflects the latest edit. The menu opens the moment an
// @-token appears under the cursor and closes when it's gone; Esc sets
// compDismissed to suppress reopening until the cursor leaves the token.
func (m *model) syncCompletion() {
	if m.picker != nil || m.help || m.pending != nil || m.question != nil {
		return
	}
	row, col := promptCursor(m.input)
	lines := strings.Split(m.input.Value(), "\n")
	line := ""
	if row >= 0 && row < len(lines) {
		line = lines[row]
	}
	query, _, ok := atToken(line, col)
	if !ok {
		m.comp = nil
		m.compDismissed = false
		return
	}
	if m.compDismissed {
		return
	}
	if m.comp == nil {
		m.comp = &completion{items: loadRepoFiles()}
	}
	m.comp.setQuery(query)
}

// acceptCompletion replaces the @-token under the cursor with the selected
// "@path " and closes the menu. The cursor lands just after the inserted path in
// the common case (token at the end of its line); a token with trailing text on
// a non-final line leaves the cursor at the input end, which is close enough.
func (m *model) acceptCompletion() {
	path, ok := m.comp.selected()
	if !ok {
		m.comp = nil
		return
	}
	row, col := promptCursor(m.input)
	lines := strings.Split(m.input.Value(), "\n")
	if row < 0 || row >= len(lines) {
		m.comp = nil
		return
	}
	runes := []rune(lines[row])
	if col > len(runes) {
		col = len(runes)
	}
	_, at, tok := atToken(lines[row], col)
	if !tok {
		m.comp = nil
		return
	}
	insert := "@" + path + " "
	lines[row] = string(runes[:at]) + insert + string(runes[col:])
	m.input.SetValue(strings.Join(lines, "\n"))
	// SetValue drops the cursor at the very end of the input. When the token sat
	// on the last line, pull it back to just after the inserted path so typing
	// continues there.
	if row == len(lines)-1 {
		m.input.SetCursor(at + len([]rune(insert)))
	}
	m.comp = nil
	m.compDismissed = false
}

// handleCompletionKey routes a key while the @-menu is open. It captures only
// navigation, accept, and dismiss; everything else (typing, backspace,
// left/right) returns handled=false so it reaches the textarea, after which
// syncCompletion re-derives the query.
func (m model) handleCompletionKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.String() {
	case "up", "ctrl+p":
		m.comp.move(-1)
		return m, nil, true
	case "down", "ctrl+n":
		m.comp.move(1)
		return m, nil, true
	case "tab", "enter":
		m.acceptCompletion()
		return m, nil, true
	case "esc":
		m.comp = nil
		m.compDismissed = true
		return m, nil, true
	}
	return m, nil, false
}

// View renders the menu as a compact bordered box anchored above the prompt: a
// title, up to completionRows path rows (windowed around the cursor), and a
// hint. Mirrors the picker's CP437 styling so it reads as one UI.
func (c *completion) View(maxW int) string {
	w := maxW - 6
	if w < 24 {
		w = 24
	}
	if w > 72 {
		w = 72
	}
	var rows []string
	if len(c.filtered) == 0 {
		rows = append(rows, cDim.Render("  (no matching files)"))
	} else {
		start := 0
		if c.cursor >= completionRows {
			start = c.cursor - completionRows + 1
		}
		end := start + completionRows
		if end > len(c.filtered) {
			end = len(c.filtered)
		}
		for i := start; i < end; i++ {
			path := c.items[c.filtered[i]]
			line := "  " + path
			if i == c.cursor {
				line = approveBar.Render(" " + path + " ")
			}
			rows = append(rows, ansi.Truncate(line, w, "…"))
		}
	}
	title := dTitle.Render(" @ files ") + "  " + cDim.Render("↑↓ move · tab/↵ insert · esc")
	body := title + "\n" + strings.Join(rows, "\n")
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colCyan).Padding(0, 1).Width(w)
	return box.Render(body)
}

// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// jumpModel renders a transcript of three prompts separated by replies tall
// enough that each prompt can reach the top of the (short) viewport.
func jumpModel() model {
	m := model{w: 60, h: 24, ready: true, follow: true}
	m.vp = newTranscriptViewport(58, 6)
	reply := strings.TrimRight(strings.Repeat("a line of reply\n", 8), "\n")
	for _, q := range []string{"first question", "second question", "third question"} {
		m.entries = append(m.entries,
			entry{kind: entUser, text: q},
			entry{kind: entClaude, text: reply})
	}
	m.rebuild()
	return m
}

// userLines returns the content line each user entry was recorded at.
func userLines(m model) []int {
	var out []int
	for i, e := range m.entries {
		if e.kind == entUser {
			out = append(out, m.entryLine[i])
		}
	}
	return out
}

// The whole feature rests on entryLine indexing real viewport lines: line
// entryLine[i] of the content must be the first line of entry i's render.
func TestEntryLineIndexesContent(t *testing.T) {
	m := jumpModel()
	lines := strings.Split(m.content.String(), "\n")
	if len(m.entryLine) != len(m.entries) {
		t.Fatalf("entryLine has %d entries, want %d", len(m.entryLine), len(m.entries))
	}
	for i, e := range m.entries {
		want := strings.Split(linkify(m.renderEntry(e)), "\n")[0]
		at := m.entryLine[i]
		if at >= len(lines) {
			t.Fatalf("entry %d: line %d past end of content (%d lines)", i, at, len(lines))
		}
		if lines[at] != want {
			t.Errorf("entry %d: content line %d = %q, want %q", i, at, lines[at], want)
		}
	}
}

// Shift+↑ walks back one prompt at a time; shift+↓ walks forward and, past the
// last prompt, returns to the live bottom.
func TestJumpPromptSteps(t *testing.T) {
	m := jumpModel()
	prompts := userLines(m)
	if len(prompts) != 3 {
		t.Fatalf("got %d prompts, want 3", len(prompts))
	}

	m.jumpPrompt(-1)
	if m.vp.YOffset != prompts[2] {
		t.Fatalf("first shift+up: offset %d, want the last prompt at %d", m.vp.YOffset, prompts[2])
	}
	if m.follow {
		t.Error("jumping up should release auto-follow")
	}
	m.jumpPrompt(-1)
	if m.vp.YOffset != prompts[1] {
		t.Fatalf("second shift+up: offset %d, want %d", m.vp.YOffset, prompts[1])
	}
	m.jumpPrompt(1)
	if m.vp.YOffset != prompts[2] {
		t.Fatalf("shift+down: offset %d, want %d", m.vp.YOffset, prompts[2])
	}
	m.jumpPrompt(1)
	if !m.vp.AtBottom() || !m.follow {
		t.Errorf("stepping past the last prompt should return to the bottom and follow (atBottom=%v follow=%v)",
			m.vp.AtBottom(), m.follow)
	}
}

// Walking off the top is a no-op, not a scroll to line 0 (entry 0 IS a prompt,
// so "above the first prompt" must find nothing rather than re-selecting it).
func TestJumpPromptStopsAtTop(t *testing.T) {
	m := jumpModel()
	for i := 0; i < 5; i++ {
		m.jumpPrompt(-1)
	}
	if got := m.vp.YOffset; got != m.entryLine[0] {
		t.Fatalf("offset %d after walking off the top, want the first prompt at %d", got, m.entryLine[0])
	}
}

// An empty transcript can't crash the binding, and the key is consumed rather
// than falling through to the textarea.
func TestJumpPromptEmpty(t *testing.T) {
	m := model{w: 60, h: 24, ready: true, follow: true}
	m.vp = newTranscriptViewport(58, 6)
	m.input = newPromptArea()
	m.jumpPrompt(-1)
	if _, _, handled := m.handleKey(tea.KeyMsg{Type: tea.KeyShiftUp}); !handled {
		t.Error("shift+up should be consumed by the jump handler")
	}
}

// /clear resets the transcript; the line index must reset with it or jumps
// would point at lines that no longer exist.
func TestJumpIndexResetsOnClear(t *testing.T) {
	m := jumpModel()
	m.entries = m.entries[:0]
	m.rebuild()
	if len(m.entryLine) != 0 || m.lineCount != 0 {
		t.Fatalf("after clear: entryLine=%d lineCount=%d, want 0/0", len(m.entryLine), m.lineCount)
	}
	m.add(entUser, "fresh start")
	if m.entryLine[0] != 0 {
		t.Errorf("first entry after clear starts at line %d, want 0", m.entryLine[0])
	}
}

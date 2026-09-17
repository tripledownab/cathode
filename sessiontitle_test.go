// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"
)

// A title replaces the first prompt as the row's label, and clearing it brings
// the first prompt back — there is no separate "unset" verb to remember.
func TestTitleReplacesTheFirstPromptInThePicker(t *testing.T) {
	s := newTestStore(t)
	const cwd = "/work/repo"
	t0 := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	s.Touch("sess-1", "sonnet", cwd, "the first thing I asked", backendClaude, t0)

	label := func() string {
		items := sessionItems(s, cwd, backendClaude)
		if len(items) != 1 {
			t.Fatalf("want one row, got %d", len(items))
		}
		return items[0].title
	}
	if got := label(); got != "the first thing I asked" {
		t.Errorf("untitled row = %q, want the first prompt", got)
	}

	s.SetTitle("sess-1", "rewriting the parser")
	if got := label(); got != "rewriting the parser" {
		t.Errorf("titled row = %q, want the title", got)
	}

	s.SetTitle("sess-1", "")
	if got := label(); got != "the first thing I asked" {
		t.Errorf("cleared row = %q, want the first prompt back", got)
	}
}

// The identifying detail moves to the second row. It is what the row is
// selected by, but not what it is scanned by.
func TestSessionRowPutsIdentityInTheSubtitle(t *testing.T) {
	s := newTestStore(t)
	const cwd = "/work/repo"
	s.Touch("sess-abcdef", "sonnet", cwd, "p", backendClaude, time.Now())
	s.SetTitle("sess-abcdef", "a name")

	it := sessionItems(s, cwd, backendClaude)[0]
	if it.title != "a name" {
		t.Errorf("title row = %q, want only the name", it.title)
	}
	for _, want := range []string{short("sess-abcdef"), "repo", "sonnet"} {
		if !strings.Contains(it.subtitle, want) {
			t.Errorf("subtitle %q should carry %q", it.subtitle, want)
		}
	}
}

// Titling a session the store has never seen would create a row with no cwd,
// model or timestamp, which then sorts and renders as a ghost.
func TestSetTitleIgnoresUnknownSessions(t *testing.T) {
	s := newTestStore(t)
	s.SetTitle("never-seen", "a name")
	if n := len(s.All()); n != 0 {
		t.Errorf("store gained %d rows from titling an unknown session", n)
	}
}

// A title is one line: the picker row is one line, and a stored newline would
// silently truncate the rest of it.
func TestTrimTitleNormalisesWhatWasTyped(t *testing.T) {
	if got := trimTitle("  spaced   out \n and wrapped "); got != "spaced out and wrapped" {
		t.Errorf("got %q", got)
	}
	if got := trimTitle("   "); got != "" {
		t.Errorf("whitespace-only should clear the title, got %q", got)
	}
	long := trimTitle(strings.Repeat("é", 200))
	if n := len([]rune(long)); n != titleMaxRunes {
		t.Errorf("capped to %d runes, want %d", n, titleMaxRunes)
	}
}

// Naming before the agent has reported a session id has nothing to attach to,
// and saying so beats appearing to save.
func TestCommitTitleRefusesWithoutASession(t *testing.T) {
	m, _ := newTestModel(t, "")
	m.session = ""

	m.commitTitle("a name")

	last := m.entries[len(m.entries)-1]
	if last.kind != entError || !strings.Contains(last.text, "no session") {
		t.Errorf("entry = %+v, want an error explaining there is nothing to name", last)
	}
}

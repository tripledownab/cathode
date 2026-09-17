// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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
	// Runes, not bytes: a title is prose, and a byte slice cuts a multi-byte
	// rune in half and emits invalid UTF-8.
	long := trimTitle(strings.Repeat("é", 200))
	if n := len([]rune(long)); n != titleMaxRunes {
		t.Errorf("capped to %d runes, want %d", n, titleMaxRunes)
	}
	if !utf8.ValidString(long) {
		t.Errorf("capped title is not valid UTF-8: %q", long)
	}
	if !strings.HasSuffix(long, "…") {
		t.Errorf("a capped title should mark what it dropped, got %q", long)
	}
}

// The same rule for the first prompt, which used to be sliced by byte offset.
func TestTruncFirstIsRuneSafe(t *testing.T) {
	got := truncFirst(strings.Repeat("é", 200))
	if !utf8.ValidString(got) {
		t.Errorf("truncated prompt is not valid UTF-8: %q", got)
	}
	if n := len([]rune(got)); n != 64 {
		t.Errorf("capped to %d runes, want 64", n)
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

// With neither a title nor a first prompt there is nothing to label the row
// with, so it falls back to the id — and the id must then not also head the
// detail row underneath it.
func TestSessionRowDoesNotPrintTheIdTwice(t *testing.T) {
	s := newTestStore(t)
	const cwd = "/work/repo"
	s.Touch("sess-abcdef", "sonnet", cwd, "", backendClaude, time.Now())

	it := sessionItems(s, cwd, backendClaude)[0]
	id := short("sess-abcdef")
	if it.title != id {
		t.Errorf("title = %q, want the id as the last-resort label", it.title)
	}
	if strings.Contains(it.subtitle, id) {
		t.Errorf("subtitle %q repeats the id already used as the title", it.subtitle)
	}
	if !strings.Contains(it.subtitle, "repo") {
		t.Errorf("subtitle %q lost the rest of the detail", it.subtitle)
	}
}

// A title has to survive the merge with claude's own session files.
//
// Every claude session has a JSONL on disk, so the filesystem entry always wins
// the merge. Copying named fields out of the store into it drops everything
// nobody remembered to list — which silently made /title a no-op on the backend
// most sessions run on, while still printing a confirmation.
func TestMergeKeepsStoreOnlyFieldsWhenTheFilesystemWins(t *testing.T) {
	s := newTestStore(t)
	const cwd = "/work/repo"
	old := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	s.Touch("sess-1", "sonnet", cwd, "the first thing I asked", backendClaude, old)
	s.SetTitle("sess-1", "rewriting the parser")

	// What listClaudeSessions produces: id, cwd, mtime, and what it could parse.
	fresh := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	fs := []sessionInfo{{ID: "sess-1", Cwd: cwd, LastUsed: fresh, First: "the first thing I asked"}}

	merged := mergeWithStore(fs, s, cwd)
	if len(merged) != 1 {
		t.Fatalf("want one merged row, got %d", len(merged))
	}
	got := merged[0]
	if got.Title != "rewriting the parser" {
		t.Errorf("Title = %q — a store-only field was dropped by the merge", got.Title)
	}
	if got.Backend != backendClaude {
		t.Errorf("Backend = %q, want it carried from the store", got.Backend)
	}
	if !got.LastUsed.Equal(fresh) {
		t.Errorf("LastUsed = %v, want the filesystem mtime to still win", got.LastUsed)
	}
	if got.Model != "sonnet" {
		t.Errorf("Model = %q, want the cached one when the file did not name it", got.Model)
	}
}

// A resumed session already knows its id, so /title must work before the first
// turn. claude does not emit system/init until a turn runs, so without seeding
// this, m.session stays empty on resume and /title refuses with "send a turn
// first" — which is what live testing hit.
func TestResumedSessionKnowsItsIdBeforeTheFirstTurn(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	m := newModel(launchConfig{
		Engine: &fakeEngine{}, Backend: backendClaude,
		Mode: "ask", Spinner: "bar", ResumeID: "resumed-1",
	})
	if m.session != "resumed-1" {
		t.Errorf("session = %q, want the id it was resumed with", m.session)
	}

	m.commitTitle("a name")
	last := m.entries[len(m.entries)-1]
	if last.kind == entError {
		t.Errorf("titling a resumed session failed: %q", last.text)
	}
	if got := titleOf(t, m.sessions, "resumed-1"); got != "a name" {
		t.Errorf("stored title = %q, want it saved", got)
	}
}

// A session started outside cathode is listed from claude's own JSONL and may
// never have been written to our store. SetTitle ignores an id it does not
// know, so the title would be dropped while the confirmation still printed.
func TestTitlingASessionTheStoreHasNeverSeen(t *testing.T) {
	m, _ := newTestModel(t, "")
	m.session = "started-elsewhere"

	m.commitTitle("named from cathode")

	if got := titleOf(t, m.sessions, "started-elsewhere"); got != "named from cathode" {
		t.Errorf("stored title = %q, want the row created and titled", got)
	}
	last := m.entries[len(m.entries)-1]
	if last.kind == entError {
		t.Errorf("unexpected error: %q", last.text)
	}
}

func titleOf(t *testing.T, s *sessionStore, id string) string {
	t.Helper()
	for _, e := range s.All() {
		if e.ID == id {
			return e.Title
		}
	}
	return ""
}

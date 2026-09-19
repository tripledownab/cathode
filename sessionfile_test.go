// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Several cathode instances run at once, each holding the store open. A write
// from one must not erase what another wrote after it loaded.
//
// This is the bug that lost session titles: /title printed its confirmation,
// and the next turn in any other window rebuilt the file from a map loaded at
// that process's start — which had never seen the row, so the row went. Every
// test before this one used a single store, which is the assumption that let it
// in.
func TestAWriteKeepsWhatAnotherInstanceWrote(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a, b := openSessionStore(), openSessionStore() // both start on an empty file

	a.Touch("sess-a", "", "/repo", "", "claude", time.Now())
	a.SetTitle("sess-a", "the one A named")

	// B has never heard of sess-a. Its own write must not take the file back to
	// the state B loaded.
	b.Touch("sess-b", "", "/repo", "", "claude", time.Now())

	fresh := openSessionStore()
	if got := fresh.entries["sess-a"].Title; got != "the one A named" {
		t.Errorf("A's title = %q, want it to survive B's write", got)
	}
	if _, ok := fresh.entries["sess-b"]; !ok {
		t.Error("B's session is missing from the file")
	}
}

// The picker must list a session started in another window. That means reading
// the file at the moment the list is built, not at process start.
func TestThePickerSeesASessionAnotherInstanceStarted(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mine, other := openSessionStore(), openSessionStore()

	other.Touch("sess-other", "", "/repo", "started elsewhere", "claude", time.Now())

	var found bool
	for _, e := range mine.All() {
		if e.ID == "sess-other" {
			found = true
		}
	}
	if !found {
		t.Error("All() did not see the other instance's session")
	}
}

// Concurrent writes from separate stores must lose nothing and corrupt nothing.
// The file is replaced wholesale on every write, so without the lock this both
// drops rows and can rename a half-written temp file into place.
func TestConcurrentWritesKeepEveryRow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	const n = 12
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := openSessionStore() // a store per instance, as in production
			id := fmt.Sprintf("sess-%02d", i)
			s.Touch(id, "", "/repo", "", "claude", time.Now())
			s.SetTitle(id, fmt.Sprintf("title %02d", i))
		}(i)
	}
	wg.Wait()

	got := openSessionStore()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("sess-%02d", i)
		e, ok := got.entries[id]
		if !ok {
			t.Errorf("%s is missing", id)
			continue
		}
		if want := fmt.Sprintf("title %02d", i); e.Title != want {
			t.Errorf("%s title = %q, want %q", id, e.Title, want)
		}
	}

	// A shared temp name is how a half-written file gets renamed into place, so
	// the name must be unique — and nothing may be left behind either way.
	if leftover, _ := filepath.Glob(filepath.Join(dir, "cathode", "*.tmp*")); len(leftover) > 0 {
		t.Errorf("temp files were not cleaned up: %v", leftover)
	}
}

// A store with no resolvable path still records, in memory. main falls back to
// one when the state dir cannot be resolved, and a session that cannot be
// persisted must not also fail to appear in this process's own picker.
func TestAStoreWithNoFileStillRecords(t *testing.T) {
	s := &sessionStore{entries: map[string]sessionInfo{}}
	s.Touch("sess-1", "", "/repo", "first", "claude", time.Now())
	s.SetTitle("sess-1", "named")

	all := s.All()
	if len(all) != 1 || all[0].Title != "named" {
		t.Errorf("All() = %+v, want the one titled session", all)
	}
}

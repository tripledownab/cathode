package main

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *sessionStore {
	t.Helper()
	return &sessionStore{
		entries: map[string]sessionInfo{},
		path:    filepath.Join(t.TempDir(), "sessions.jsonl"),
	}
}

// TestSessionTouchUpsertsAndBumps pins that Touch creates a new entry and that
// a follow-up Touch with empty fields preserves prior metadata while still
// bumping LastUsed — the picker depends on that ordering.
func TestSessionTouchUpsertsAndBumps(t *testing.T) {
	s := newTestStore(t)
	t0 := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	s.Touch("abc123", "sonnet", "/work/repo", "fix the bug", t0)

	got := s.entries["abc123"]
	if got.Model != "sonnet" || got.Cwd != "/work/repo" || got.First != "fix the bug" {
		t.Fatalf("initial Touch lost metadata: %+v", got)
	}

	// Empty-field follow-up: must not overwrite existing values, must bump time.
	t1 := t0.Add(5 * time.Minute)
	s.Touch("abc123", "", "", "", t1)
	got = s.entries["abc123"]
	if got.Model != "sonnet" || got.Cwd != "/work/repo" || got.First != "fix the bug" {
		t.Fatalf("follow-up Touch clobbered metadata: %+v", got)
	}
	if !got.LastUsed.Equal(t1) {
		t.Fatalf("LastUsed not bumped: got %v want %v", got.LastUsed, t1)
	}
}

// TestSessionAllSortsByRecency pins that the picker gets newest-first ordering.
func TestSessionAllSortsByRecency(t *testing.T) {
	s := newTestStore(t)
	s.Touch("older", "m", "/a", "p1", time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC))
	s.Touch("middle", "m", "/b", "p2", time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	s.Touch("newest", "m", "/c", "p3", time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC))

	got := s.All()
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	if got[0].ID != "newest" || got[1].ID != "middle" || got[2].ID != "older" {
		t.Fatalf("order = %v, want [newest middle older]", []string{got[0].ID, got[1].ID, got[2].ID})
	}
}

// TestSessionItemsFiltersByCwd pins that the picker only shows sessions
// recorded against the caller's current directory — running doorway in
// /work/repoA must not surface sessions from /work/repoB.
func TestSessionItemsFiltersByCwd(t *testing.T) {
	s := newTestStore(t)
	t0 := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	s.Touch("here1", "m", "/work/repoA", "p", t0)
	s.Touch("here2", "m", "/work/repoA/", "p", t0) // trailing slash → same after Clean
	s.Touch("other", "m", "/work/repoB", "p", t0)
	s.Touch("legacy", "m", "", "p", t0) // pre-cwd entry

	items := sessionItems(s, "/work/repoA")
	if len(items) != 2 {
		t.Fatalf("len=%d, want 2 (here1, here2); got %+v", len(items), items)
	}
	ids := map[string]bool{}
	for _, it := range items {
		ids[it.id] = true
	}
	if !ids["here1"] || !ids["here2"] {
		t.Fatalf("missing expected ids: %+v", items)
	}
	if ids["other"] || ids["legacy"] {
		t.Fatalf("unwanted ids leaked through filter: %+v", items)
	}

	// Empty cwd disables the filter — used by tests that don't care about it.
	if len(sessionItems(s, "")) != 4 {
		t.Fatalf("empty-cwd filter should return all entries")
	}
}

// TestBuildResumeArgvDropsExistingResume pins that re-exec doesn't accumulate
// stale -resume flags when chaining picks across sessions.
func TestBuildResumeArgvDropsExistingResume(t *testing.T) {
	in := []string{"/bin/doorway", "-mode", "ask", "-resume", "old-id", "-spinner", "shade"}
	out := buildResumeArgv(in, "new-id")
	want := []string{"/bin/doorway", "-mode", "ask", "-spinner", "shade", "-resume", "new-id"}
	if !sameArgv(out, want) {
		t.Fatalf("out = %v\nwant %v", out, want)
	}
}

// TestBuildResumeArgvAppendsWhenAbsent pins the first-time path.
func TestBuildResumeArgvAppendsWhenAbsent(t *testing.T) {
	in := []string{"/bin/doorway", "-mode", "ask"}
	out := buildResumeArgv(in, "fresh-id")
	want := []string{"/bin/doorway", "-mode", "ask", "-resume", "fresh-id"}
	if !sameArgv(out, want) {
		t.Fatalf("out = %v\nwant %v", out, want)
	}
}

func sameArgv(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeFakeSession plants one of claude's session JSONLs under a fake home so
// listClaudeSessions has something to enumerate. mtime is bumped explicitly
// because os.WriteFile uses "now" and the tests need a deterministic order.
func writeFakeSession(t *testing.T, home, cwd, id, body string, mtime time.Time) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", projectSlug(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// TestListClaudeSessionsReadsFromProjectDir pins the load-bearing fix: the
// picker must surface sessions that live in ~/.claude/projects/<slug>/, not
// just ones doorway's own store has recorded. Sessions started by running
// `claude` directly were invisible to the picker before this change.
func TestListClaudeSessionsReadsFromProjectDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := "/work/repoA"

	older := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)

	// Array-form content (what claude emits for prompts the user typed in the
	// UI) plus an assistant turn so we can also pick up the model name.
	writeFakeSession(t, home, cwd, "alpha", `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello there"}]}}
{"type":"assistant","message":{"role":"assistant","model":"claude-opus-4-7","content":[{"type":"text","text":"hi"}]}}
`, older)

	// Bare-string content (an older format claude has also emitted) — make
	// sure we still extract the prompt.
	writeFakeSession(t, home, cwd, "beta", `{"type":"user","message":{"role":"user","content":"fix the bug"}}
`, newer)

	// A session in a different project — must not leak into repoA's results.
	writeFakeSession(t, home, "/work/repoB", "gamma", `{"type":"user","message":{"role":"user","content":"other"}}
`, newer)

	got := listClaudeSessions(cwd)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2; got=%+v", len(got), got)
	}
	// Newest-first ordering.
	if got[0].ID != "beta" || got[1].ID != "alpha" {
		t.Fatalf("order=%v, want [beta alpha]", []string{got[0].ID, got[1].ID})
	}
	if got[0].First != "fix the bug" {
		t.Fatalf("beta first prompt = %q, want %q", got[0].First, "fix the bug")
	}
	if got[1].First != "hello there" {
		t.Fatalf("alpha first prompt = %q, want %q", got[1].First, "hello there")
	}
	if got[1].Model != "claude-opus-4-7" {
		t.Fatalf("alpha model = %q, want %q", got[1].Model, "claude-opus-4-7")
	}
}

// TestSessionItemsMergesClaudeAndStore pins that the picker merges the
// filesystem-derived list with doorway's own store: claude-only sessions
// surface, store metadata enriches them where it exists, and store-only
// entries for the same cwd are still listed.
func TestSessionItemsMergesClaudeAndStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := "/work/repoA"

	writeFakeSession(t, home, cwd, "fromclaude", `{"type":"user","message":{"role":"user","content":"hi"}}
`, time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC))

	s := newTestStore(t)
	// Store-only entry for the same cwd — must still show up.
	s.Touch("storeonly", "sonnet", cwd, "prior prompt", time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC))
	// Different cwd — must be filtered out.
	s.Touch("elsewhere", "sonnet", "/work/repoB", "other", time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC))

	items := sessionItems(s, cwd)
	ids := map[string]bool{}
	for _, it := range items {
		ids[it.id] = true
	}
	if !ids["fromclaude"] {
		t.Fatalf("filesystem session missing from picker: %+v", items)
	}
	if !ids["storeonly"] {
		t.Fatalf("store-only session missing from picker: %+v", items)
	}
	if ids["elsewhere"] {
		t.Fatalf("other-cwd store entry leaked through: %+v", items)
	}
}

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadPriorTranscriptSeedsContext verifies the resume path projects entries
// and pulls ctxTokens from the most recent assistant usage (input + both cache
// counts) — the number that seeds the context gauge so a resumed session isn't
// stuck at 0%.
func TestLoadPriorTranscriptSeedsContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".claude", "projects", projectSlug(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// A user turn, an older assistant turn, then a newer one — the newest
	// assistant usage must win (131 + 168 + 84387 = 84686).
	lines := "" +
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"older"}],"usage":{"input_tokens":10,"cache_read_input_tokens":20,"cache_creation_input_tokens":0,"output_tokens":5}}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"newer"}],"usage":{"input_tokens":131,"cache_read_input_tokens":84387,"cache_creation_input_tokens":168,"output_tokens":876}}}` + "\n"
	id := "sess-abc"
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, ctxTok := loadPriorTranscript(id, 40)
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}
	if ctxTok != 84686 {
		t.Fatalf("ctxTokens = %d, want 84686 (newest assistant usage)", ctxTok)
	}
}

// TestObserveCtxGrowsLimit checks the shared seed/live helper: observing a
// context size past the current tier snaps the limit up, so a resumed >200K
// session reports a sane percentage instead of an over-100% gauge.
func TestObserveCtxGrowsLimit(t *testing.T) {
	m := &model{ctxLimit: 200_000}
	m.observeCtx(640_000)
	if m.ctxTokens != 640_000 {
		t.Fatalf("ctxTokens = %d, want 640000", m.ctxTokens)
	}
	if m.ctxLimit != 1_000_000 {
		t.Fatalf("ctxLimit = %d, want 1000000 (grown past 640K)", m.ctxLimit)
	}
}

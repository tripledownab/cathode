// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProjectSlugMatchesClaude pins the naming rule for the path shapes that
// used to break it. Every one of these used to resolve to a path that does not
// exist, which is what emptied the resume picker and the replayed transcript
// for those projects.
func TestProjectSlugMatchesClaude(t *testing.T) {
	cases := []struct{ cwd, want string }{
		{"/Users/w/Work/Cathode/cathode", "-Users-w-Work-Cathode-cathode"},
		{"/Users/w/Work/Billing Service/web", "-Users-w-Work-Billing-Service-web"},           // space
		{"/Users/w/.config/wezterm", "-Users-w--config-wezterm"},                             // dot: two dashes, no collapsing
		{"/Users/w/Work/api-gateway/rate_limiter", "-Users-w-Work-api-gateway-rate-limiter"}, // underscore
		{"/Users/w/Work/Data Platform", "-Users-w-Work-Data-Platform"},                       // space, no subdir
	}
	for _, c := range cases {
		if got := projectSlug(c.cwd); got != c.want {
			t.Errorf("projectSlug(%q) = %q, want %q", c.cwd, got, c.want)
		}
	}
}

// TestClaudeProjectDirFallsBackToRecordedCwd covers the case the slug rule
// can't: a directory claude named by some rule we don't reproduce. The cwd
// stamped inside the session records is the ground truth, so resume still
// finds it.
func TestClaudeProjectDirFallsBackToRecordedCwd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := "/work/café.ai"

	// Deliberately NOT projectSlug(cwd) — this stands in for any naming rule
	// we'd otherwise miss. The non-ASCII rune is the point: projectSlug would
	// turn it into a dash, and this directory keeps it.
	dir := filepath.Join(home, ".claude", "projects", "-work-café-ai")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Head-of-file bookkeeping carries no cwd, exactly like the real thing.
	body := `{"type":"queue-operation","operation":"enqueue"}
{"type":"user","cwd":"/work/caf` + "é" + `.ai","message":{"role":"user","content":"hello"}}
`
	if err := os.WriteFile(filepath.Join(dir, "sess.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := claudeProjectDir(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("claudeProjectDir = %q, want %q (found via recorded cwd)", got, dir)
	}

	// And an unrelated project must not be claimed by the scan.
	other, err := claudeProjectDir("/work/somewhere-else")
	if err != nil {
		t.Fatal(err)
	}
	if other == dir {
		t.Fatalf("unrelated cwd matched %q", dir)
	}
}

// TestClaudeProjectDirPrefersSlug keeps the scan off the common path: when the
// slugged directory exists it wins outright, no directory walk.
func TestClaudeProjectDirPrefersSlug(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := "/work/repo A"

	want := filepath.Join(home, ".claude", "projects", projectSlug(cwd))
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}
	// A decoy whose records claim the same cwd — the slug match must win.
	decoy := filepath.Join(home, ".claude", "projects", "aaa-decoy")
	if err := os.MkdirAll(decoy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(decoy, "s.jsonl"), []byte(`{"cwd":"/work/repo A"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := claudeProjectDir(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("claudeProjectDir = %q, want %q", got, want)
	}
}

// TestLoadPriorTranscriptSpacedCwd is the end-to-end regression: a project whose
// path contains a space replays its history instead of coming back empty.
func TestLoadPriorTranscriptSpacedCwd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// loadPriorTranscript reads os.Getwd(), so put the test in a real spaced dir.
	work := filepath.Join(t.TempDir(), "Billing Service", "web")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
	// Take the cwd back from the OS: on macOS the temp dir resolves through a
	// symlink (/var → /private/var), and it's the resolved path that has to slug.
	work, err = os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(home, ".claude", "projects", projectSlug(work))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"user","message":{"role":"user","content":"first prompt"}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"a reply"}]}}
`
	if err := os.WriteFile(filepath.Join(dir, "sess.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, _ := loadPriorTranscript("sess", 40)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (spaced cwd must still resolve)", len(entries))
	}
	if entries[0].kind != entUser || entries[0].text != "first prompt" {
		t.Fatalf("entry 0 = %+v, want the bare-string user prompt", entries[0])
	}
}

// TestContentBlocksBothEncodings pins the decoder that the replay and the
// picker now share: a bare string is a text block, an array passes through.
func TestContentBlocksBothEncodings(t *testing.T) {
	got := contentBlocks([]byte(`"just text"`))
	if len(got) != 1 || got[0].Type != "text" || got[0].Text != "just text" {
		t.Fatalf("string content = %+v", got)
	}
	got = contentBlocks([]byte(`[{"type":"text","text":"hi"},{"type":"tool_use","name":"Edit"}]`))
	if len(got) != 2 || got[1].Name != "Edit" {
		t.Fatalf("array content = %+v", got)
	}
	if b := contentBlocks([]byte(`"   "`)); len(b) != 0 {
		t.Fatalf("blank string content = %+v, want none", b)
	}
	if b := contentBlocks([]byte(`null`)); len(b) != 0 {
		t.Fatalf("null content = %+v, want none", b)
	}
}

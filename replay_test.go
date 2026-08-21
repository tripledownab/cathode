package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReplayUserText covers the projection of one user text block: real prompts
// pass through, the CLI's tag records become the line the live run shows, and
// pure bookkeeping is dropped.
func TestReplayUserText(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		rkind replayKind
		kind  entryKind
		want  string
	}{
		{"prompt", "fix the parser", replayShow, entUser, "fix the parser"},
		{"empty", "   \n", replaySkip, entUser, ""},
		{
			"slash command",
			"<command-name>/compact</command-name>\n            <command-message>compact</command-message>\n            <command-args></command-args>",
			replayShow, entUser, "/compact",
		},
		{
			"slash command with args",
			"<command-message>model</command-message>\n<command-name>/model</command-name>\n<command-args>opus[1m]</command-args>",
			replayShow, entUser, "/model opus[1m]",
		},
		{
			"command stdout",
			"<local-command-stdout>Set model to \x1b[1mOpus 4.8\x1b[22m</local-command-stdout>",
			replayStdout, entInfo, "Set model to Opus 4.8",
		},
		{
			"empty command stdout",
			"<local-command-stdout></local-command-stdout>",
			replaySkip, entUser, "",
		},
		{
			"task notification",
			"<task-notification>\n<task-id>bzcqxd1m7</task-id>\n<status>completed</status>\n</task-notification>",
			replaySkip, entUser, "",
		},
		{
			// A prompt that quotes a tag is the user's own words. Only a record
			// that is nothing but wrappers counts as bookkeeping.
			"prompt quoting a tag",
			"we get the tags raw, e.g. <local-command-stdout>Compacted </local-command-stdout> — why?",
			replayShow, entUser, "we get the tags raw, e.g. <local-command-stdout>Compacted </local-command-stdout> — why?",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, k := replayUserText(c.in)
			if k != c.rkind {
				t.Fatalf("replayKind = %d, want %d", k, c.rkind)
			}
			if k == replaySkip {
				return
			}
			if e.kind != c.kind {
				t.Errorf("kind = %d, want %d", e.kind, c.kind)
			}
			if e.text != c.want {
				t.Errorf("text = %q, want %q", e.text, c.want)
			}
		})
	}
}

// TestLoadPriorTranscriptHidesMeta is the reported bug: resuming a compacted
// session opened on the raw summary, the caveat and the <command-name> markup.
// The replay must show the compaction as its one-line marker instead.
func TestLoadPriorTranscriptHidesMeta(t *testing.T) {
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

	lines := "" +
		`{"type":"user","message":{"role":"user","content":"commit to 81"}}` + "\n" +
		`{"type":"user","isCompactSummary":true,"isVisibleInTranscriptOnly":true,"message":{"role":"user","content":"This session is being continued...\n## 8. Current Work\n</summary>"}}` + "\n" +
		`{"type":"user","isMeta":true,"message":{"role":"user","content":"<local-command-caveat>Caveat: The messages below...</local-command-caveat>"}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":"<command-name>/compact</command-name>\n            <command-args></command-args>"}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":"<local-command-stdout>Compacted </local-command-stdout>"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"picking up #42"}]}}` + "\n"
	id := "sess-meta"
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, _ := loadPriorTranscript(id, 40)
	want := []entry{
		{kind: entUser, text: "commit to 81"},
		{kind: entUser, text: "/compact"},
		{kind: entInfo, text: compactDoneText},
		{kind: entClaude, text: "picking up #42"},
	}
	if len(entries) != len(want) {
		t.Fatalf("entries = %d, want %d: %+v", len(entries), len(want), entries)
	}
	for i, w := range want {
		if entries[i].kind != w.kind || entries[i].text != w.text {
			t.Errorf("entry %d = (%d, %q), want (%d, %q)", i, entries[i].kind, entries[i].text, w.kind, w.text)
		}
	}
}

// TestLoadPriorTranscriptAutoCompact covers the compaction nobody asked for:
// there is no /compact record to hold the marker back for, so it must land
// between the prompt that triggered it and the reply that followed.
func TestLoadPriorTranscriptAutoCompact(t *testing.T) {
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

	lines := "" +
		`{"type":"user","message":{"role":"user","content":"carry on"}}` + "\n" +
		`{"type":"user","isCompactSummary":true,"message":{"role":"user","content":"This session is being continued..."}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"on it"}]}}` + "\n"
	id := "sess-auto"
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, _ := loadPriorTranscript(id, 40)
	want := []entry{
		{kind: entUser, text: "carry on"},
		{kind: entInfo, text: compactDoneText},
		{kind: entClaude, text: "on it"},
	}
	if len(entries) != len(want) {
		t.Fatalf("entries = %d, want %d: %+v", len(entries), len(want), entries)
	}
	for i, w := range want {
		if entries[i].kind != w.kind || entries[i].text != w.text {
			t.Errorf("entry %d = (%d, %q), want (%d, %q)", i, entries[i].kind, entries[i].text, w.kind, w.text)
		}
	}
}

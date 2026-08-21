package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// writePrompt puts text in the (temp) state dir's prompt file.
func writePrompt(t *testing.T, text string) {
	t.Helper()
	p := sysPromptPath()
	if p == "" {
		t.Fatal("no state dir")
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// noDefaultPrompt blanks the shipped default so a test can exercise the
// nothing-configured state, which no longer happens out of the box.
func noDefaultPrompt(t *testing.T) {
	t.Helper()
	orig := defaultSysPrompt
	t.Cleanup(func() { defaultSysPrompt = orig })
	defaultSysPrompt = ""
}

func TestSysPromptFile(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	noDefaultPrompt(t)

	if got := loadSysPrompt(); got != "" {
		t.Errorf("no file yet: got %q, want empty", got)
	}
	writePrompt(t, "  Prefer short answers.\nNo emoji.  \n")
	if got := loadSysPrompt(); got != "Prefer short answers.\nNo emoji." {
		t.Errorf("loadSysPrompt = %q (should be trimmed, body intact)", got)
	}
	// Whitespace-only is nothing to append, not a prompt.
	writePrompt(t, "\n\n   \n")
	if got := loadSysPrompt(); got != "" {
		t.Errorf("whitespace-only file: got %q, want empty", got)
	}
	if got := sysPromptPath(); filepath.Base(got) != sysPromptFile {
		t.Errorf("prompt path %q should end in %q", got, sysPromptFile)
	}
}

// The summary drives the picker subtitle: first line, marked when there's more,
// and a pointer to the file when there's nothing yet.
func TestSysPromptSummary(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	noDefaultPrompt(t)

	if got := sysPromptSummary(); !strings.Contains(got, sysPromptFile) {
		t.Errorf("empty summary should point at the file, got %q", got)
	}
	writePrompt(t, "Be terse.")
	if got := sysPromptSummary(); got != "Be terse." {
		t.Errorf("single line: got %q", got)
	}
	writePrompt(t, "Be terse.\nAlso: no emoji.")
	if got := sysPromptSummary(); got != "Be terse. …" {
		t.Errorf("multi-line should be marked, got %q", got)
	}
	writePrompt(t, strings.Repeat("x", 200))
	if got := sysPromptSummary(); len([]rune(got)) > 50 {
		t.Errorf("long first line should be truncated, got %d runes", len([]rune(got)))
	}
	// A pasted prompt is prose: em-dashes and curly quotes are normal, and a
	// byte-offset cut lands mid-rune and emits invalid UTF-8. The 45 ASCII
	// chars put the em-dash across the old [:47] boundary exactly.
	writePrompt(t, strings.Repeat("a", 45)+"—tail of the instruction line goes here")
	got := sysPromptSummary()
	if !utf8.ValidString(got) {
		t.Errorf("summary must stay valid UTF-8, got %q", got)
	}
}

// The shipped default must actually seed the file, and be usable as a prompt.
// As a const this branch could never run in a test — it would have executed
// for the first time on a user's machine.
func TestDefaultSysPromptSeeds(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	orig := defaultSysPrompt
	t.Cleanup(func() { defaultSysPrompt = orig })
	defaultSysPrompt = "Seeded instructions."

	if got := loadSysPrompt(); got != "Seeded instructions." {
		t.Errorf("first load should seed from the default, got %q", got)
	}
	b, err := os.ReadFile(sysPromptPath())
	if err != nil {
		t.Fatalf("seeding should have written the file: %v", err)
	}
	if string(b) != "Seeded instructions." {
		t.Errorf("seeded file = %q", string(b))
	}
	// A user edit wins over the default from then on.
	writePrompt(t, "My own rules.")
	if got := loadSysPrompt(); got != "My own rules." {
		t.Errorf("edited file should win, got %q", got)
	}

	// The real shipped default must be non-empty and valid, or /sysprompt on
	// would refuse to enable out of the box.
	if strings.TrimSpace(orig) == "" {
		t.Error("shipped default prompt is empty")
	}
	if !utf8.ValidString(orig) {
		t.Error("shipped default prompt is not valid UTF-8")
	}
}

// What main actually hands the subprocess. The flag is only passed when it
// would do something — claude rejects --append-system-prompt-file on a missing
// path, so an enabled-but-empty toggle must not add it.
func TestSysPromptArgs(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	noDefaultPrompt(t)

	if got := sysPromptArgs(false); got != nil {
		t.Errorf("toggle off: got %v, want no flag", got)
	}
	if got := sysPromptArgs(true); got != nil {
		t.Errorf("toggle on but no prompt file: got %v, want no flag", got)
	}
	writePrompt(t, "Be terse.")
	got := sysPromptArgs(true)
	if len(got) != 2 || got[0] != "--append-system-prompt-file" {
		t.Fatalf("got %v, want the file flag and a path", got)
	}
	if b, err := os.ReadFile(got[1]); err != nil || strings.TrimSpace(string(b)) != "Be terse." {
		t.Errorf("flag should point at the readable prompt file (err=%v)", err)
	}
}

// Toggling persists and asks for the restart that actually applies it — but
// only when a restart would achieve something.
func TestCommitSysPrompt(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	writePrompt(t, "Be terse.")

	// Turning it on with a live session restarts, resuming that session so the
	// conversation survives.
	m := &model{session: "sess-1"}
	cmd := m.commitSysPrompt(sysPromptOn)
	if cmd == nil {
		t.Fatal("enabling should return tea.Quit to trigger the re-exec")
	}
	if m.resumeID != "sess-1" {
		t.Errorf("resumeID = %q, want the live session so the restart resumes it", m.resumeID)
	}
	if !m.settings.SysPrompt {
		t.Error("setting should be persisted as on")
	}
	if loadSettings().SysPrompt != true {
		t.Error("setting should survive to disk")
	}

	// Selecting the state it's already in, with the file unchanged since launch,
	// must not restart — but it must still say so.
	same := &model{session: "sess-1", settings: settings{SysPrompt: true}, sysPromptSeen: "Be terse."}
	if cmd := same.commitSysPrompt(sysPromptOn); cmd != nil || same.resumeID != "" {
		t.Error("re-selecting the current state should be a no-op, not a restart")
	}
	if len(same.entries) != 1 {
		t.Fatalf("declining to restart must say why, got %d entries", len(same.entries))
	}

	// Same selection, but the file changed since launch. claude read it at
	// launch, so re-selecting "on" is the only way to apply the edit.
	writePrompt(t, "Be terse. Answer in one line.")
	edited := &model{session: "sess-1", settings: settings{SysPrompt: true}, sysPromptSeen: "Be terse."}
	if cmd := edited.commitSysPrompt(sysPromptOn); cmd == nil || edited.resumeID != "sess-1" {
		t.Error("an edited prompt file should restart to reload it")
	}
	if !edited.settings.SysPrompt {
		t.Error("reloading an edit must leave the toggle on")
	}

	// Mid-turn: persist, but don't kill the running turn.
	busy := &model{session: "sess-1", busy: true}
	if cmd := busy.commitSysPrompt(sysPromptOn); cmd != nil || busy.resumeID != "" {
		t.Error("should not restart while busy")
	}
	if !busy.settings.SysPrompt {
		t.Error("busy path should still persist the setting")
	}

	// No session id yet — restarting would drop the conversation instead of
	// resuming it.
	fresh := &model{}
	if cmd := fresh.commitSysPrompt(sysPromptOn); cmd != nil || fresh.resumeID != "" {
		t.Error("should not restart without a session to resume")
	}
}

// Enabling an empty prompt would restart for no effect — the worst kind of
// no-op, since it looks like the toggle is broken.
func TestCommitSysPromptEmpty(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	noDefaultPrompt(t)

	m := &model{session: "sess-1"}
	cmd := m.commitSysPrompt(sysPromptOn)
	if cmd != nil || m.resumeID != "" {
		t.Error("enabling with no prompt text should not restart")
	}
	if m.settings.SysPrompt {
		t.Error("enabling with no prompt text should not flip the setting")
	}
	if len(m.entries) != 1 || m.entries[0].kind != entError {
		t.Fatalf("should explain why nothing happened, got %d entries", len(m.entries))
	}
	if !strings.Contains(m.entries[0].text, sysPromptFile) {
		t.Errorf("error should say where to write the prompt, got %q", m.entries[0].text)
	}

	// Turning it *off* with no text is fine — nothing to validate.
	off := &model{session: "sess-1", settings: settings{SysPrompt: true}}
	if cmd := off.commitSysPrompt(sysPromptOff); cmd == nil {
		t.Error("disabling should restart regardless of the file's contents")
	}
}

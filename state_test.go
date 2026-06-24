package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMigrateLegacyState verifies the one-time doorway → cathode rename: when
// only the legacy dir exists, stateDir relocates it (data and all) to the new
// name on first lookup.
func TestMigrateLegacyState(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_STATE_HOME", base)

	old := filepath.Join(base, legacyStateName)
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "sessions.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := stateDir()
	want := filepath.Join(base, appStateName)
	if dir != want {
		t.Fatalf("stateDir() = %q, want %q", dir, want)
	}
	if _, err := os.Stat(filepath.Join(want, "sessions.jsonl")); err != nil {
		t.Fatalf("migrated data missing: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("legacy dir should be gone, stat err = %v", err)
	}
}

// TestMigrateLegacyStateNoClobber ensures an existing cathode dir is never
// overwritten by a stale doorway dir — the new dir wins and the old is left
// untouched for the user to clean up manually.
func TestMigrateLegacyStateNoClobber(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_STATE_HOME", base)

	old := filepath.Join(base, legacyStateName)
	cur := filepath.Join(base, appStateName)
	for _, d := range []string{old, cur} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cur, "settings.json"), []byte(`{"theme":"nord"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stateDir()

	b, err := os.ReadFile(filepath.Join(cur, "settings.json"))
	if err != nil {
		t.Fatalf("cathode settings clobbered: %v", err)
	}
	if string(b) != `{"theme":"nord"}` {
		t.Fatalf("cathode settings changed: %q", b)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("legacy dir should be left intact, stat err = %v", err)
	}
}

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSettingsRoundTrip pins the XDG path logic and the save→load cycle,
// including the empty-field fallback to the default style.
func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	if got := loadSettings(); got.Header != headerCyan {
		t.Fatalf("default Header = %q, want %q", got.Header, headerCyan)
	}

	saveSettings(settings{Header: headerRainbow})
	if got := loadSettings(); got.Header != headerRainbow {
		t.Fatalf("after save, Header = %q, want %q", got.Header, headerRainbow)
	}
	if _, err := os.Stat(filepath.Join(dir, "cathode", "settings.json")); err != nil {
		t.Fatalf("settings.json not written: %v", err)
	}

	// An all-empty file is also what a settings.json written before a field
	// existed looks like — every field must fall back, not land on "".
	saveSettings(settings{Header: ""})
	got := loadSettings()
	if got.Header != headerCyan {
		t.Fatalf("empty Header should fall back to %q, got %q", headerCyan, got.Header)
	}
	if got.Bar != barComet {
		t.Fatalf("settings.json predating the bar field should fall back to %q, got %q", barComet, got.Bar)
	}
}

// TestRenderHeaderEveryStyle ensures every row in the /settings picker maps to a
// renderHeader branch that produces output (no orphaned ids).
func TestRenderHeaderEveryStyle(t *testing.T) {
	for _, s := range headerStyles {
		if out := renderHeader(s.id, "DOORWAY", 3); out == "" {
			t.Errorf("renderHeader(%q) returned empty", s.id)
		}
	}
}

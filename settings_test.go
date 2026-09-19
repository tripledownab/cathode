// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

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

	saveSettings(func(s *settings) { s.Header = headerRainbow })
	if got := loadSettings(); got.Header != headerRainbow {
		t.Fatalf("after save, Header = %q, want %q", got.Header, headerRainbow)
	}
	path := filepath.Join(dir, "cathode", "settings.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("settings.json not written: %v", err)
	}

	// An all-empty file is what a settings.json written before a field existed
	// looks like — every field must fall back, not land on "". Written directly,
	// because saveSettings now merges into the defaults and so cannot produce one.
	if err := os.WriteFile(path, []byte(`{"header":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadSettings()
	if got.Header != headerCyan {
		t.Fatalf("empty Header should fall back to %q, got %q", headerCyan, got.Header)
	}
	if got.Bar != barComet {
		t.Fatalf("settings.json predating the bar field should fall back to %q, got %q", barComet, got.Bar)
	}
}

// A theme carries whether the banner shows, so both must land in the file
// together. Written one at a time, the file can hold this theme with the previous
// one's banner — and the next launch draws chrome the theme does not want.
func TestCommittingAThemePersistsItsBanner(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	m := newModel(launchConfig{Engine: &fakeEngine{}, Mode: "ask", Spinner: "bar"})

	m.commitTheme("cinder") // the one bannerless theme
	if got := loadSettings(); got.Theme != "cinder" || got.Banner != bannerOff {
		t.Errorf("stored (%q, %q), want (cinder, %q)", got.Theme, got.Banner, bannerOff)
	}

	// And back: every other theme states that it wants the banner, so switching
	// away has to bring it back rather than leaving it hidden.
	m.commitTheme(defaultTheme)
	if got := loadSettings(); got.Theme != defaultTheme || got.Banner != bannerOn {
		t.Errorf("stored (%q, %q), want (%q, %q)", got.Theme, got.Banner, defaultTheme, bannerOn)
	}
}

// A setting changed in one window must not revert one changed in another. Each
// window holds a copy loaded at its own start, so writing that copy back was
// what reverted the other — the bug sessionStore documents, in settings.json.
func TestSavingOneSettingKeepsAnotherWindowsChange(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a := newModel(launchConfig{Engine: &fakeEngine{}, Mode: "ask", Spinner: "bar"})
	b := newModel(launchConfig{Engine: &fakeEngine{}, Mode: "ask", Spinner: "bar"})

	a.commitSetting(func(s *settings) { s.Diff = diffSplit })
	// B loaded before that and still holds the default diff style. Changing its
	// theme must write the theme, not B's whole copy.
	b.commitSetting(func(s *settings) { s.Theme = "cinder" })

	got := loadSettings()
	if got.Diff != diffSplit {
		t.Errorf("Diff = %q, want %q kept across the other window's write", got.Diff, diffSplit)
	}
	if got.Theme != "cinder" {
		t.Errorf("Theme = %q, want cinder", got.Theme)
	}
	// And B keeps its own view: a setting changed elsewhere must not restyle a
	// running window mid-session.
	if b.settings.Diff == diffSplit {
		t.Error("B's in-memory diff style followed the file; a running window keeps its own")
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

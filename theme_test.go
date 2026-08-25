// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestThemesHavePalettes ensures every row in the /settings theme picker maps to
// a real palette (no orphaned ids).
func TestThemesHavePalettes(t *testing.T) {
	for _, th := range themes {
		if _, ok := palettes[th.id]; !ok {
			t.Errorf("theme %q listed in picker but has no palette", th.id)
		}
	}
}

// TestApplyThemeSwapsAndRebuilds confirms applyTheme repoints the active palette
// and rebuilds a representative style; unknown ids fall back to the default.
func TestApplyThemeSwapsAndRebuilds(t *testing.T) {
	defer applyTheme(defaultTheme) // restore for other tests

	applyTheme("dracula")
	if colCyan != palettes["dracula"].cyan {
		t.Fatalf("colCyan = %v, want %v", colCyan, palettes["dracula"].cyan)
	}
	if got := cName.GetForeground(); got != palettes["dracula"].cyan {
		t.Fatalf("cName foreground = %v, want %v", got, palettes["dracula"].cyan)
	}

	applyTheme("does-not-exist")
	if colCyan != palettes[defaultTheme].cyan {
		t.Fatalf("unknown theme should fall back to %s", defaultTheme)
	}
}

// TestEveryThemeStatesItsBanner is the regression for a one-way toggle.
// The prop used to be optional, so only cinder spoke: switching to it hid
// the banner and switching away left it hidden. Every theme must state its
// banner so the switch works in both directions.
func TestEveryThemeStatesItsBanner(t *testing.T) {
	withBanner, without := 0, 0
	for _, th := range themes {
		if th.banner {
			withBanner++
		} else {
			without++
		}
	}
	if without != 1 {
		t.Errorf("%d themes hide the banner, want only cinder", without)
	}
	if withBanner != len(themes)-1 {
		t.Errorf("themes showing the banner = %d, want %d", withBanner, len(themes)-1)
	}
}

// TestBannerForRoundTrips covers the accessor commitTheme uses, in both
// directions, plus the unknown-id case.
func TestBannerForRoundTrips(t *testing.T) {
	if got := bannerFor("cinder"); got != bannerOff {
		t.Errorf("bannerFor(cinder) = %q, want %q", got, bannerOff)
	}
	for _, id := range []string{"bbs", "dracula", "rosepine"} {
		if got := bannerFor(id); got != bannerOn {
			t.Errorf("bannerFor(%s) = %q, want %q", id, got, bannerOn)
		}
	}
	if got := bannerFor("does-not-exist"); got != bannerOn {
		t.Errorf("bannerFor(unknown) = %q, want %q — a missing theme must not remove chrome", got, bannerOn)
	}
}

// TestHeaderStylesAreAnimationsOnly keeps the two concepts apart. "hidden" was
// retired as an animation; leaving it in the picker would offer two different
// ways to hide the banner, one of which no theme could see.
func TestHeaderStylesAreAnimationsOnly(t *testing.T) {
	for _, s := range headerStyles {
		if s.id == headerHidden {
			t.Fatal("headerHidden is still offered as a header animation")
		}
	}
}

// TestLoadSettingsMigratesHiddenHeader covers an old settings.json written
// while "hidden" was an animation value.
func TestLoadSettingsMigratesHiddenHeader(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "cathode"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"header":"hidden","theme":"cinder","fps":12}`
	if err := os.WriteFile(filepath.Join(dir, "cathode", "settings.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	s := loadSettings()
	if s.Banner != bannerOff {
		t.Errorf("Banner = %q, want %q — the old hidden header meant no banner", s.Banner, bannerOff)
	}
	if s.Header == headerHidden {
		t.Error("Header kept the retired hidden value")
	}
}

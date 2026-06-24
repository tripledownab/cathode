package main

import "testing"

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

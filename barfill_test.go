// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestBarFillForRoundTrips covers the accessor applyTheme uses, in both
// directions, plus the unknown-id case.
func TestBarFillForRoundTrips(t *testing.T) {
	if got := barFillFor("cinder"); got != barsFlat {
		t.Errorf("barFillFor(cinder) = %v, want barsFlat", got)
	}
	for _, id := range []string{"bbs", "dracula", "rosepine"} {
		if got := barFillFor(id); got != barsFilled {
			t.Errorf("barFillFor(%s) = %v, want barsFilled", id, got)
		}
	}
	if got := barFillFor("does-not-exist"); got != barsFilled {
		t.Errorf("barFillFor(unknown) = %v, want barsFilled — a missing theme must not restyle the chrome", got)
	}
}

// TestOnlyCinderIsFlat is the counterpart of TestOnlyCinderHidesTheBanner: the
// flat row is cinder's whole point, and a second theme picking it up silently
// would be a mistake, not a feature.
func TestOnlyCinderIsFlat(t *testing.T) {
	for _, th := range themes {
		want := barsFilled
		if th.id == "cinder" {
			want = barsFlat
		}
		if th.bars != want {
			t.Errorf("theme %s has bars %v, want %v", th.id, th.bars, want)
		}
	}
}

// TestFlatThemeStatusRowHasNoFill is the regression guard for the whole
// treatment: the row must paint no background of its own, and the two things
// that carry color on it — the context gauge and the busy indicator — must
// still emit theirs. The fill was never what colored them.
func TestFlatThemeStatusRowHasNoFill(t *testing.T) {
	defer restoreTheme(t)()
	applyTheme("cinder")

	// 176k of 200k = 88%, past the ⚠ threshold, so the gauge is red.
	row := bbsStatus("build", "opus", "sess1234", "main", 0.01, 176_000, 1200, 200_000, true, false, "⣾", 100)

	if bg := bgSGR(colCyan); strings.Contains(row, bg) {
		t.Errorf("flat status row paints the cyan fill (%s):\n%q", bg, row)
	}
	// Not even the palette's own black: stamping it would hide a transparent or
	// image terminal background behind an opaque band.
	if bg := bgSGR(colBlack); strings.Contains(row, bg) {
		t.Errorf("flat status row paints an opaque background (%s):\n%q", bg, row)
	}
	if fg := fgSGR(colRed); !strings.Contains(row, fg) {
		t.Errorf("flat status row lost the red gauge at 88%% (%s):\n%q", fg, row)
	}
	if fg := fgSGR(colAccent); !strings.Contains(row, fg) {
		t.Errorf("flat status row lost the ember busy indicator (%s):\n%q", fg, row)
	}
}

// TestFilledThemeStatusRowKeepsItsFill is the other direction: switching away
// from cinder must restore the inverted row, not leave the flat one behind.
func TestFilledThemeStatusRowKeepsItsFill(t *testing.T) {
	defer restoreTheme(t)()
	applyTheme("cinder")
	applyTheme("gruvbox") // a hex palette; bbs uses ANSI indices, which emit no 48;2

	row := bbsStatus("build", "opus", "sess1234", "main", 0.01, 24_000, 1200, 200_000, false, false, "", 100)
	if bg := bgSGR(colCyan); !strings.Contains(row, bg) {
		t.Errorf("filled status row lost its fill (%s):\n%q", bg, row)
	}
}

// TestStatusReadingOrder: the label/value split is what gives the flat row
// structure once the fill is gone. On a filled row the two must collapse to the
// same style — dark-on-the-fill is the only readable pairing there.
func TestStatusReadingOrder(t *testing.T) {
	defer restoreTheme(t)()

	applyTheme("cinder")
	if sbarLabel.GetForeground() == sbarValue.GetForeground() {
		t.Error("flat theme: label and value share a foreground, so the row has no reading order")
	}
	applyTheme("gruvbox")
	if sbarLabel.GetForeground() != sbarValue.GetForeground() {
		t.Error("filled theme: label and value must collapse to the same inverted style")
	}
}

// restoreTheme forces TrueColor so styles emit explicit 24-bit SGR, and puts
// both the profile and the active theme back when the test ends.
func restoreTheme(t *testing.T) func() {
	t.Helper()
	prof := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	return func() {
		lipgloss.SetColorProfile(prof)
		applyTheme(defaultTheme)
	}
}

// fgSGR and bgSGR return the SGR run lipgloss emits for one palette color, by
// asking lipgloss to render a probe rather than rebuilding the sequence from
// the hex. termenv round-trips a hex color through a float color space, so
// #F05A28 comes back as 240;89;40, not 240;90;40 — reconstructing it by hand
// looks right and fails.
//
// Matching the run rather than the whole escape is also deliberate: lipgloss
// merges bold, foreground and background into one sequence.
func fgSGR(c lipgloss.Color) string { return sgrRun(lipgloss.NewStyle().Foreground(c), "38;2;") }
func bgSGR(c lipgloss.Color) string { return sgrRun(lipgloss.NewStyle().Background(c), "48;2;") }

func sgrRun(style lipgloss.Style, prefix string) string {
	probe := style.Render("x")
	i := strings.Index(probe, prefix)
	if i < 0 {
		panic(fmt.Sprintf("no %q in probe %q — is the color profile truecolor?", prefix, probe))
	}
	j := strings.Index(probe[i:], "m")
	return probe[i : i+j]
}

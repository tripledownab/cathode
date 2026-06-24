package main

// Reproducible regeneration of the marketing SVGs in assets/. The images are
// rendered from the LIVE UI code (splashScreen + the real model.renderBackground),
// so they can't silently drift from the app again. The ANSI->SVG engine lives in
// ansisvg_test.go. Opt-in so a plain `go test` never rewrites checked-in files:
//
//	CATHODE_GENASSETS=1 go test -run TestGenerateAssets
//
// It forces lipgloss into TrueColor so every style emits explicit 24-bit SGR.

import (
	"os"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestGenerateAssets(t *testing.T) {
	if os.Getenv("CATHODE_GENASSETS") == "" {
		t.Skip("set CATHODE_GENASSETS=1 to regenerate assets/cathode-*.svg")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	// Render the marketing assets in Catppuccin Mocha rather than the default
	// near-monochrome amber/cyan BBS phosphor palette, so the images show off the
	// colored chrome (diff red/green, mauve approve bar, teal status bar).
	applyTheme("catppuccin")

	for _, a := range []struct {
		path, screen string
	}{
		{"assets/cathode-splash.svg", genSplashScreen()},
		{"assets/cathode-preview.svg", genPreviewScreen()},
	} {
		// Canvas + default text from the active palette (Catppuccin base #1e1e2e
		// and text #cdd6f4) so unstyled cells sit on-theme, not on BBS black/white.
		if err := os.WriteFile(a.path, []byte(ansiToSVG(a.screen, string(colBlack), string(colWhite))), 0o644); err != nil {
			t.Fatalf("write %s: %v", a.path, err)
		}
		t.Logf("wrote %s", a.path)
	}
}

// genSplashScreen renders the boot screen at its final reveal frame. height 0
// returns the bare content (no vertical centering), so the image is compact.
func genSplashScreen() string {
	return splashScreen(90, 0, splashFinalFrame, 0)
}

// genPreviewScreen drives a real model through renderBackground with a small
// sample turn: a user request, a plain reply, an Edit diff card, and the live
// approval bar — i.e. exactly what the app paints in ask mode.
func genPreviewScreen() string {
	sp := spinner.New()
	sp.Spinner = bbsSpinner("scan")
	m := model{
		mode: "ask", session: "a1b2c3d4", modelID: "sonnet",
		// "theme" header shimmer ties the wordmark to the active palette (teal in
		// Catppuccin) instead of the default hardcoded cyan, for a cohesive image.
		headerStyle: headerTheme,
		lastCost:    0.0042,
		ctxTokens:   24000, outTokens: 1200, ctxLimit: 200000,
		busy: true, follow: true, ready: true,
		sp:      sp,
		pending: &approvalReq{toolName: "Edit"},
		w:       92,
	}
	m.entries = []entry{
		{kind: entUser, text: "refactor add() to take a third arg and update the caller"},
		{kind: entClaude, text: "Here's the change to both the function and its caller:"},
		{kind: entDiff, diffs: []fileDiff{{
			file: "math.go",
			old:  "func add(a, b int) int {\n\treturn a + b\n}",
			new:  "func add(a, b, c int) int {\n\treturn a + b + c\n}",
		}}},
	}
	// Fit the viewport to the transcript so there's no dead space in the image.
	m.vp = newTranscriptViewport(m.w-1, 100)
	m.rebuild()
	m.h = m.vp.TotalLineCount() + 6 // banner(3)+divider(1)+prompt(1)+status(1)
	m.resizeViewport()
	m.rebuild()
	return m.renderBackground()
}

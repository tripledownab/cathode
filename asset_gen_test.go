// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

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
	"encoding/json"
	"os"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
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
		{"assets/cathode-preview.svg", genPreviewScreen(bannerOn)},
		{"assets/cathode-question.svg", genQuestionScreen()},
	} {
		// Canvas + default text from the active palette (Catppuccin base #1e1e2e
		// and text #cdd6f4) so unstyled cells sit on-theme, not on BBS black/white.
		if err := os.WriteFile(a.path, []byte(ansiToSVG(a.screen, hexOf(colBlack), hexOf(colWhite))), 0o644); err != nil {
			t.Fatalf("write %s: %v", a.path, err)
		}
		t.Logf("wrote %s", a.path)
	}
}

// TestGenerateThemeAssets renders the ask-mode preview scene once per color
// theme into assets/themes/<id>.svg, so the docs can show every palette without
// a live capture. Same opt-in as the marketing assets:
//
//	CATHODE_GENASSETS=1 go test -run TestGenerateThemeAssets
//
// hexOf resolves the canvas/default colors so the BBS palette's ANSI indices
// ("0"/"15") become real hex; the basic-SGR path in ansisvg_test.go does the
// same for its body, so the neon-on-black theme renders like every other one.
func TestGenerateThemeAssets(t *testing.T) {
	if os.Getenv("CATHODE_GENASSETS") == "" {
		t.Skip("set CATHODE_GENASSETS=1 to regenerate assets/themes/*.svg")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	if err := os.MkdirAll("assets/themes", 0o755); err != nil {
		t.Fatalf("mkdir assets/themes: %v", err)
	}
	for _, th := range themes {
		applyTheme(th.id)
		svg := ansiToSVG(genPreviewScreen(bannerFor(th.id)), hexOf(colBlack), hexOf(colWhite))
		path := "assets/themes/" + th.id + ".svg"
		if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s", path)
	}
}

// genSplashScreen renders the boot screen at its final reveal frame. height 0
// returns the bare content (no vertical centering), so the image is compact.
func genSplashScreen() string {
	return splashScreen(90, 0, splashFinalFrame, 0)
}

// previewModel builds a real model mid-turn — a user request, a plain reply,
// and an Edit diff card — sized so the viewport hugs its transcript. Both the
// ask-mode preview (approval bar) and the question-modal scene share it; each
// scene supplies its own foreground.
//
// banner is the caller's bannerOn/bannerOff, so a theme that drops the wordmark
// is shown dropping it. Passing it in rather than reading the persisted setting
// keeps the images independent of whatever the developer has selected.
func previewModel(banner string) model {
	sp := spinner.New()
	sp.Spinner = bbsSpinner("scan")
	ta := textarea.New()
	ta.SetWidth(88)
	ta.SetHeight(1)
	m := model{
		mode: "ask", session: "a1b2c3d4", modelID: "sonnet",
		// "theme" header shimmer ties the wordmark to the active palette (teal in
		// Catppuccin) instead of the default hardcoded cyan, for a cohesive image.
		headerStyle: headerTheme,
		lastCost:    0.0042,
		ctxTokens:   24000, outTokens: 1200, ctxLimit: 200000,
		busy: true, follow: true, ready: true,
		sp:    sp,
		input: ta,
		w:     92,
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
	m.settings.Banner = banner
	m.vp = newTranscriptViewport(m.w-1, 100)
	m.rebuild()
	m.h = m.vp.TotalLineCount() + 2 + m.topChromeRows() // + prompt(1) + status(1)
	m.resizeViewport()
	m.rebuild()
	return m
}

// genPreviewScreen paints exactly what the app shows in ask mode: the sample
// turn plus the live approval bar in place of the prompt.
func genPreviewScreen(banner string) string {
	m := previewModel(banner)
	m.pending = &approvalReq{toolName: "Edit"}
	return m.renderBackground()
}

// genQuestionScreen floats the AskUserQuestion modal over the same diff scene —
// Claude pausing mid-edit to ask which signature to use. It mirrors View()'s
// modal path (placeOverlay of the picker on renderBackground), so the image
// can't drift from how the app actually stacks a question over the transcript.
// A few extra rows give the box some transcript to float over.
func genQuestionScreen() string {
	m := previewModel(bannerOn)
	m.rebuild()

	in, _ := json.Marshal(askInput{Questions: []askQuestion{{
		Question: "How should add() take the third argument?",
		Header:   "signature",
		Options: []askOption{
			{Label: "Required positional", Description: "add(a, b, c int) — update every caller"},
			{Label: "Variadic", Description: "add(a int, rest ...int) — sum any number of args"},
			{Label: "Options struct", Description: "add(Args{...}) — future-proof, more verbose"},
		},
	}}})
	q, ok := parseAskQuestion(approvalReq{toolName: askUserQuestionTool, input: in})
	if !ok {
		return m.renderBackground()
	}
	return placeOverlay(m.renderBackground(), q.picker(m.w, m.h).View(), m.w, m.h)
}

package main

import (
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// splashTickMsg drives the boot-screen reveal animation. We keep it private
// to the splash so other tick streams (spinner, etc.) can't be confused with
// it.
type splashTickMsg struct{}

// splashTick fires the next reveal frame after a short delay. Re-issued from
// the Update loop until splashFrame hits splashFinalFrame.
func splashTick() tea.Cmd {
	return tea.Tick(140*time.Millisecond, func(time.Time) tea.Msg { return splashTickMsg{} })
}

// splashFinalFrame is the last frame index used by splashScreen below — once
// the model reaches this frame, the animation rests until dismissal.
const splashFinalFrame = 7

// pickLogoIdx chooses which wide wordmark variant this launch shows (logos.go).
// math/rand is auto-seeded on Go 1.20+, so it varies per run; the model picks
// once at startup so the choice is stable across re-renders.
func pickLogoIdx() int {
	if len(logoVariants) <= 1 {
		return 0
	}
	return rand.Intn(len(logoVariants))
}

// splashLogo returns the wordmark rows: the randomly-chosen wide variant when it
// fits the terminal, else the compact fallback so it never overflows.
func splashLogo(idx, width int) []string {
	if idx >= 0 && idx < len(logoVariants) {
		if art := logoVariants[idx]; artWidth(art) <= width {
			return strings.Split(art, "\n")
		}
	}
	return strings.Split(logoCompact, "\n")
}

// artWidth is the widest display row in a multi-line art block.
func artWidth(s string) int {
	w := 0
	for _, l := range strings.Split(s, "\n") {
		if lw := lipgloss.Width(l); lw > w {
			w = lw
		}
	}
	return w
}

// centerBlock pads a multi-row art block uniformly so its columns stay aligned,
// then applies a style to each row.
func centerBlock(lines []string, width int, style lipgloss.Style) []string {
	w := 0
	for _, l := range lines {
		if lw := lipgloss.Width(l); lw > w {
			w = lw
		}
	}
	lp := (width - w) / 2
	if lp < 0 {
		lp = 0
	}
	pad := strings.Repeat(" ", lp)
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = pad + style.Render(l)
	}
	return out
}

// splashScreen is the boot/login screen: a (randomly-chosen) wordmark, a faux
// modem handshake, a scene divider, a SAUCE-style credit, and a logon prompt.
// Sections reveal one frame at a time so the boot looks like a real modem
// negotiation; the first keypress dismisses it regardless of progress.
func splashScreen(width, height, frame, logoIdx int) string {
	if width < 44 {
		width = 44
	}
	center := func(s string) string { return lipgloss.PlaceHorizontal(width, lipgloss.Center, s) }

	lines := []string{"", ""}
	if frame >= 1 {
		lines = append(lines, centerBlock(splashLogo(logoIdx, width), width, hdrName)...)
	}
	if frame >= 2 {
		lines = append(lines,
			"",
			center(hdrDeco.Render("░▒▓█ ")+hdrSub.Render(flavor("a one-node board"))+hdrDeco.Render(" █▓▒░")),
			"",
		)
	}
	if frame >= 3 {
		lines = append(lines, center(cDim.Render("ATDT 1-800-CLAUDE . . .")))
	}
	if frame >= 4 {
		lines = append(lines, center(dAdd.Render("CONNECT 57600/ARQ/V.42bis/LAPM")))
	}
	if frame >= 5 {
		lines = append(lines, center(cName.Render(flavor("CARRIER DETECTED"))))
	}
	if frame >= 6 {
		lines = append(lines,
			"",
			sceneDivider(appName, width),
			"",
			center(cDim.Render(flavor("sysop: william")+"   "+ornBullet+"   "+flavor("est 2026")+"   "+ornBullet+"   "+flavor("node 1/1"))),
			"",
		)
	}
	if frame >= splashFinalFrame {
		lines = append(lines, center(approveBar.Render(" "+flavor("press [ENTER] to logon")+" . . . ")))
	}
	body := strings.Join(lines, "\n")
	if height > len(lines) {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, body)
	}
	return body
}

// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
)

// bbsBanner renders the top wordmark with ░▒▓█ gradient-block flourishes. The
// wordmark itself animates per the chosen header style (set via /settings),
// its color band drifting with phase (incremented by rainbowTickMsg in the
// update loop).
func bbsBanner(width, phase int, style string) string {
	if width < 24 {
		width = 24
	}
	title := fmt.Sprintf("%s %s %s   %s",
		hdrDeco.Render("░▒▓█"),
		renderHeader(style, studly(appName), phase),
		hdrDeco.Render("█▓▒░"),
		hdrSub.Render(ornDeco+" "+flavor("Claude on your Max plan")+" "+ornDeco))
	return hdrBox.Width(width - 2).Render(title)
}

// bbsScrollbar renders a vertical scrollbar `height` rows tall. Mirrors Crush's
// algorithm: thumb size scales with the visible portion; thumb position scales
// linearly with the scroll offset. When content fits, returns a blank track so
// the layout column stays stable.
func bbsScrollbar(height, contentSize, viewportSize, offset int, visible bool) string {
	if height <= 0 {
		return ""
	}
	// Auto-hidden (see scroll.go): a blank 1-col column keeps the layout width
	// stable — no reflow — while leaving only spaces to the right of the text, so
	// a drag-select copies clean lines instead of a column of │ pipes.
	if !visible {
		rows := make([]string, height)
		for i := range rows {
			rows[i] = " "
		}
		return strings.Join(rows, "\n")
	}
	if contentSize <= viewportSize {
		rows := make([]string, height)
		for i := range rows {
			rows[i] = scrollTrack.Render("│")
		}
		return strings.Join(rows, "\n")
	}
	thumbSize := maxInt(1, height*viewportSize/contentSize)
	maxOffset := contentSize - viewportSize
	trackSpace := height - thumbSize
	thumbPos := 0
	if trackSpace > 0 && maxOffset > 0 {
		thumbPos = minInt(trackSpace, offset*trackSpace/maxOffset)
	}
	var sb strings.Builder
	for i := 0; i < height; i++ {
		if i > 0 {
			sb.WriteString("\n")
		}
		if i >= thumbPos && i < thumbPos+thumbSize {
			sb.WriteString(scrollThumb.Render("┃"))
		} else {
			sb.WriteString(scrollTrack.Render("│"))
		}
	}
	return sb.String()
}

// ---- ASCII spinners (shown in the status bar while Claude is working) ----
// Period-flavoured throbbers. Pick via the -spinner flag.
func bbsSpinner(name string) spinner.Spinner {
	fast := time.Second / 8
	switch name {
	case "shade": // CP437 shade pulse — the most ANSI-authentic
		return spinner.Spinner{Frames: []string{"░", "▒", "▓", "█", "▓", "▒"}, FPS: time.Second / 6}
	case "block": // rotating quadrant
		return spinner.Spinner{Frames: []string{"▖", "▘", "▝", "▗"}, FPS: fast}
	case "arrow":
		return spinner.Spinner{Frames: []string{"►", "▻", "▷", "▻"}, FPS: fast}
	case "scan": // knight-rider lightbar
		return spinner.Spinner{Frames: []string{"[█▒▒▒]", "[▒█▒▒]", "[▒▒█▒]", "[▒▒▒█]", "[▒▒█▒]", "[▒█▒▒]"}, FPS: time.Second / 10}
	case "bar": // classic spinning bar
		fallthrough
	default:
		return spinner.Spinner{Frames: []string{"|", "/", "-", "\\"}, FPS: fast}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

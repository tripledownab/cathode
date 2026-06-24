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

// pendingTray renders the queued-while-busy messages as a dim strip above the
// input. Returns "" when the queue is empty. Truncates each line to width and
// caps the visible count so the tray doesn't take over the viewport.
func pendingTray(queue []string, width int) string {
	if len(queue) == 0 {
		return ""
	}
	const maxShown = 5
	rows := []string{cDim.Render(fmt.Sprintf("▶▶▶ %d queued", len(queue)))}
	shown := queue
	if len(shown) > maxShown {
		shown = shown[:maxShown]
	}
	for _, q := range shown {
		line := strings.ReplaceAll(q, "\n", " ")
		rows = append(rows, cDim.Render("  ▸ "+trunc(line, width-4)))
	}
	if len(queue) > maxShown {
		rows = append(rows, cDim.Render(fmt.Sprintf("  … %d more", len(queue)-maxShown)))
	}
	return strings.Join(rows, "\n")
}

// bbsScrollbar renders a vertical scrollbar `height` rows tall. Mirrors Crush's
// algorithm: thumb size scales with the visible portion; thumb position scales
// linearly with the scroll offset. When content fits, returns a blank track so
// the layout column stays stable.
func bbsScrollbar(height, contentSize, viewportSize, offset int) string {
	if height <= 0 {
		return ""
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

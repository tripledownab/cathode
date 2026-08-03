package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// placeOverlay pastes `fg` onto `bg` centered in a (termW × termH) field, so
// the surrounding transcript stays visible. Lines outside the fg's vertical
// region are returned untouched; inside the region the bg line is sliced at
// the fg's horizontal extent and the fg cells take over the middle.
//
// lipgloss v1 has no overlay primitive, so we do the splice ourselves using
// x/ansi for cell-accurate cuts that preserve SGR state across the seam.
func placeOverlay(bg, fg string, termW, termH int) string {
	fgW, fgH := overlaySize(fg)
	x := (termW - fgW) / 2
	if x < 0 {
		x = 0
	}
	y := (termH - fgH) / 2
	if y < 0 {
		y = 0
	}
	return placeOverlayAt(bg, fg, x, y)
}

// placeOverlayAt pastes fg onto bg with its top-left at (x, y). The centered
// placeOverlay is the common case; the inline @-file menu uses this directly to
// anchor itself just above the prompt.
func placeOverlayAt(bg, fg string, x, y int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")
	fgW, fgH := overlaySize(fg)
	// Pad bg vertically so the overlay can land even if bg has fewer rows than
	// the terminal (rare, but possible on a short transcript).
	for len(bgLines) < y+fgH {
		bgLines = append(bgLines, "")
	}
	for i, fgLine := range fgLines {
		row := y + i
		if row < 0 {
			continue
		}
		bgLines[row] = spliceLine(bgLines[row], fgLine, x, fgW)
	}
	return strings.Join(bgLines, "\n")
}

// overlaySize measures fg by its widest line (so the box stays axis-aligned even
// when inner rows differ in trailing whitespace) and its line count.
func overlaySize(fg string) (w, h int) {
	lines := strings.Split(fg, "\n")
	for _, l := range lines {
		if lw := lipgloss.Width(l); lw > w {
			w = lw
		}
	}
	return w, len(lines)
}

// spliceLine produces `left | fg | right` where left is the first `x` visible
// cells of bg, fg is the centered overlay row, and right is everything in bg
// past column `x+fgW`. ANSI escape state is preserved on both sides because
// ansi.Truncate / ansi.TruncateLeft re-emit the active SGR sequence at the cut.
func spliceLine(bg, fg string, x, fgW int) string {
	bgW := lipgloss.Width(bg)

	// Left slice: first x cells of bg, padded with spaces if bg is shorter
	// than x (so the fg lands at the intended column).
	left := ansi.Truncate(bg, x, "")
	if pad := x - lipgloss.Width(left); pad > 0 {
		left += strings.Repeat(" ", pad)
	}

	// Right slice: skip the first x+fgW cells of bg. TruncateLeft with prefix
	// "" returns the cells from column n onwards with state preserved.
	right := ""
	if cutAt := x + fgW; cutAt < bgW {
		right = ansi.TruncateLeft(bg, cutAt, "")
	}

	return left + fg + right
}

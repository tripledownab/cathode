// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Wrap mode for the picker.
//
// A question's options carry a description that explains the choice, and a
// one-line row cut it off at the box edge, which left the user choosing
// without the explanation. In wrap mode the title has its own row and the
// description wraps onto as many rows under it as it needs. Items therefore
// differ in height, so the visible window is measured in rows, not items.

// wrapIndent is how far a description sits in from the box edge, under its title.
const wrapIndent = "    "

// wrapItemLines is one item as unstyled lines: the title, then the wrapped
// description. Each line fits in width cells.
func (p *picker) wrapItemLines(idx, width int) []string {
	it := p.items[idx]
	lines := []string{"  " + p.mark(idx) + it.title}
	textW := width - len(wrapIndent)
	if textW < 1 || strings.TrimSpace(it.subtitle) == "" {
		return lines
	}
	for _, l := range strings.Split(ansi.Wrap(it.subtitle, textW, ""), "\n") {
		lines = append(lines, wrapIndent+l)
	}
	return lines
}

// wrapWindow picks the items to draw so the cursor item is visible and the
// drawn rows fit in maxRows. It walks back from the cursor as far as the rows
// allow, then fills forward. heights holds the row count of each filtered item.
func wrapWindow(heights []int, cursor, maxRows int) (start, end int) {
	start, used := cursor, heights[cursor]
	for start > 0 && used+heights[start-1] <= maxRows {
		start--
		used += heights[start]
	}
	end = cursor + 1
	for end < len(heights) && used+heights[end] <= maxRows {
		used += heights[end]
		end++
	}
	return start, end
}

// wrapRows renders the visible items in wrap mode, each line padded to listW.
// It returns the rows and the window of filtered items they show.
func (p *picker) wrapRows(listW, maxRows int) (rows []string, start, end int) {
	// The selected item renders inside the lightbar's padding, so it wraps to
	// the narrower width. Wrap every item to that width, so an item keeps the
	// same height when the cursor moves onto it and the window does not jump.
	inner := listW - approveBar.GetHorizontalFrameSize()
	if inner < 1 {
		inner = 1
	}
	items := make([][]string, len(p.filtered))
	heights := make([]int, len(p.filtered))
	for i, idx := range p.filtered {
		items[i] = p.wrapItemLines(idx, inner)
		heights[i] = len(items[i])
	}
	start, end = wrapWindow(heights, p.cursor, maxRows)
	for i := start; i < end; i++ {
		lines := items[i]
		// One item taller than the whole window is cut, not drawn past the box.
		if len(lines) > maxRows {
			lines = append(lines[:maxRows-1:maxRows-1], lines[maxRows-1]+"…")
		}
		for j, l := range lines {
			switch {
			case i == p.cursor:
				rows = append(rows, approveBar.Render(padRow(l, inner)))
			case j == 0:
				rows = append(rows, padRow(l, listW))
			default:
				rows = append(rows, padRow(cDim.Render(l), listW))
			}
		}
	}
	return rows, start, end
}

// padRow truncates line to w cells and pads it with spaces to exactly w, so
// the scrollbar lands as a straight column whatever the styled content is.
// "…" rather than a bare cut: a row trimmed to fit should say so, or a
// truncated title reads as the whole of a shorter one.
func padRow(line string, w int) string {
	line = ansi.Truncate(line, w, "…")
	if n := w - lipgloss.Width(line); n > 0 {
		line += strings.Repeat(" ", n)
	}
	return line
}

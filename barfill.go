// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/charmbracelet/lipgloss"

// barFill is how a theme paints its bars.
//
// The BBS look inverts them: dark text on a solid color, for the status row and
// the diff card's filename chip. A flat theme drops those fills and carries the
// same information as colored text on the canvas, so nothing but the lightbar
// is a filled surface.
//
// The lightbar is deliberately excluded. That same style marks the selected row
// in every picker (picker.go, complete.go, splash.go), and a selection needs a
// fill to read as one — flattening it would leave a menu with no visible focus.
type barFill int

const (
	barsFilled barFill = iota // dark text on a solid status row and filename chip
	barsFlat                  // colored text straight on the canvas
)

// themeBars is the active theme's treatment. applyTheme sets it before
// buildStyles runs; read it at style-build time only, never per frame.
var themeBars barFill

// barFillFor returns the bar treatment a theme asks for. An unknown id keeps
// the filled bars, mirroring applyTheme's fallback to the default palette: a
// missing theme should not silently restyle the chrome.
func barFillFor(themeID string) barFill {
	for _, t := range themes {
		if t.id == themeID {
			return t.bars
		}
	}
	return barsFilled
}

// diffTitleStyle is the diff card's filename chip: a filled tab, or bold light
// text on the canvas when the theme is flat.
func diffTitleStyle() lipgloss.Style {
	if themeBars == barsFlat {
		return lipgloss.NewStyle().Bold(true).Foreground(colWhite).Padding(0, 1)
	}
	return lipgloss.NewStyle().Bold(true).Foreground(colBlack).Background(colCyan).Padding(0, 1)
}

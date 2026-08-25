// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Status-bar palette and the pieces of the row that are painted rather than
// composed. bbsStatus (status.go) says what the row contains; this file says
// how it looks.
//
// A filled row must set its background on every segment: lipgloss inner styles
// emit a full SGR reset at end-of-chunk, so any nested styled run drops the
// fill and the text after it would render on the terminal default until the
// next SGR. Re-asserting bg on every chunk keeps the fill contiguous.
//
// A flat row sets no background at all — not even the palette's own black. The
// reset-to-default that breaks a filled row is exactly what a flat one wants:
// the terminal's background shows through, so the row inherits a transparent
// or image background instead of stamping an opaque band over it.
//
// sbarLabel and sbarValue are the row's reading order. On a filled row they are
// the same inverted style, because dark-on-the-fill is the only readable
// pairing there. On a flat row the label dims to the ash grey and the value
// stays light, which is what gives the row structure once the fill is gone.
var (
	sbarBase  lipgloss.Style // separators, padding, and the context gauge's empty track
	sbarLabel lipgloss.Style // MDL, M0D3, N0D3, CTX, OUT
	sbarValue lipgloss.Style // what each of those reads
	sbarBusy  lipgloss.Style // the spinner + WORKING, while a turn runs
	sbarGreen lipgloss.Style
	sbarYel   lipgloss.Style
	sbarRed   lipgloss.Style
)

// buildStatusStyles rebuilds the status-bar styles from the active palette and
// the theme's bar treatment. Called from buildStyles (theme.go) on startup and
// every theme change.
func buildStatusStyles() {
	if themeBars == barsFlat {
		// Foreground only: the row is drawn straight onto the terminal. The
		// gauge and the spinner keep their color — the fill was never what
		// carried it — and only the gauge's empty track dims, to the same grey
		// the scrollbar and the /compact comet already use for a track.
		flat := lipgloss.NewStyle()
		sbarBase = flat.Foreground(colGray)
		sbarLabel = flat.Foreground(colGray)
		sbarValue = flat.Foreground(colWhite)
		sbarBusy = flat.Foreground(colAccent).Bold(true)
		sbarGreen = flat.Foreground(colGreen)
		sbarYel = flat.Foreground(colYel)
		sbarRed = flat.Foreground(colRed).Bold(true)
		return
	}
	inv := lipgloss.NewStyle().Bold(true).Background(colCyan)
	sbarBase = inv.Foreground(colBlack)
	sbarLabel, sbarValue, sbarBusy = sbarBase, sbarBase, sbarBase
	sbarGreen = inv.Foreground(colGreen)
	sbarYel = inv.Foreground(colYel)
	sbarRed = inv.Foreground(colRed)
}

// sbarSeg renders one "LABEL value" pair. Both styles collapse to the same one
// on a filled theme, so the row composes identically either way.
func sbarSeg(label, value string) string {
	return sbarLabel.Render(label) + sbarBase.Render(" ") + sbarValue.Render(value)
}

// ctxBar is the 8-cell gradient progress bar shown next to the CTX % in the
// status line. Filled blocks shift color as pressure rises: green → yellow →
// red, matching the ⚠ threshold used elsewhere. Empty cells render as ░ in the
// base style, so the track sits on the row's background whether that's the
// solid fill or the canvas.
func ctxBar(pct int) string {
	const w = 8
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := w * pct / 100
	style := sbarGreen
	switch {
	case pct >= 80:
		style = sbarRed
	case pct >= 60:
		style = sbarYel
	}
	return style.Render(strings.Repeat("█", filled)) + sbarBase.Render(strings.Repeat("░", w-filled))
}

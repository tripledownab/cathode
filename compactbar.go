// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ---- /compact progress bar layout ----
//
// One full terminal row: throbber, shade pulse, label, animated track, elapsed
// clock. The track styles themselves live in compactstyle.go; the state that
// decides whether the row is up at all lives in compact.go.
//
// Everything here is per-frame chrome, drawn straight into renderBackground —
// never a transcript entry, which would force a full rerender() at 10fps (see
// CLAUDE.md). The one hard invariant: the row must never exceed `width` cells,
// or the frame wraps and shunts the status bar off screen.

// The typical-duration hint, shown once the wait is long enough to read as a
// hang. It is a population figure, not a prediction about this run, and is
// worded that way on purpose — there is no progress signal to predict from (see
// the header comment), so it must never become a percentage.
//
// Measured over 72 distinct compactions in local session logs plus a live
// probe: min 33s, p25 108s, median 120s, p75 138s, max 308s. Duration grows far
// slower than context — 23K took 33s, 108K took 87s, 1M took 113s — because the
// cost is generating the summary, not reading the input. So it's bucketed
// rather than scaled: one flat figure would be badly wrong at the small end,
// and anything finer would be reading precision into a 4x spread.
const (
	compactHintAfter = 20 * time.Second
	compactSmallCtx  = 200_000
)

// compactHintFor is the note for a compaction of the given pre-compact context
// size. Zero (size unknown) takes the larger estimate — better to over-warn
// than to make a two-minute wait look overdue.
func compactHintFor(preTokens int) string {
	if preTokens > 0 && preTokens < compactSmallCtx {
		return " · ~1m typical"
	}
	return " · ~2m typical"
}

// compactBar renders the full-width row:
//
//	/ ▒▓█ C0Mp4C71nG C0n73X7 ░░░░▒▓██▓▒░░░░░░░░░░░░░░░░░░   12s
//
// Left to right: the session's throbber (whatever `-spinner` picked, so the two
// animations on screen match), a CP437 shade wave marching into the label, the
// track, and the elapsed clock. spin is the already-rendered spinner frame;
// style picks the track animation (compactstyle.go, set via /settings or /bar).
func compactBar(width int, elapsed time.Duration, spin, style string, preTokens int) string {
	if width < 8 {
		// Nothing meaningful fits. A blank row still holds its line, which is what
		// the layout budgeted for; anything else would wrap and push the status
		// bar off screen.
		return ""
	}
	phase := int(elapsed.Seconds() * compactFPS)
	// Fixed-width clock: right-aligned in a 4-cell field so the track doesn't
	// lose a cell (and the comet jump) when the count rolls 9s → 10s.
	clock := fmt.Sprintf("%3ds", int(elapsed.Seconds()))
	wave := shadeWave(phase)

	// Segment widths, in the order they're shed as the terminal narrows: the
	// track goes first, then the throbber, then half the label. The clock and
	// the pulse are the last things standing — they're what say "still alive".
	// Widths go through lipgloss.Width, not len: the hint carries a multi-byte
	// ornament, and the labels would be a byte-count trap the moment one grows
	// a non-ASCII character.
	const lead = 1 // the leading space, aligning with the status bar's
	waveW := 3 + 1
	head, headW := "", 0
	if spin != "" {
		head, headW = cmpComet.Render(spin)+" ", lipgloss.Width(spin)+1
	}
	long, short := flavor("compacting context"), flavor("compacting")
	longW, shortW := lipgloss.Width(long), lipgloss.Width(short)
	// fixed is everything but the label and the track.
	fixedFor := func(c string) int { return lead + headW + waveW + 1 + lipgloss.Width(c) }

	// The "~2m typical" hint only earns its cells once the wait has gone on long
	// enough to look like a hang, and only if a usable track survives it — on a
	// narrow terminal the motion is worth more than the note.
	if elapsed >= compactHintAfter {
		if hinted := clock + compactHintFor(preTokens); width-fixedFor(hinted)-longW-1 >= 8 {
			clock = hinted
		}
	}
	fixed := fixedFor(clock)

	row := func(label string, trackW int) string {
		s := " " + head + wave + " " + cmpLabel.Render(label)
		if trackW >= 4 {
			s += " " + barTrack(style, trackW, phase)
		}
		return s + " " + cmpTime.Render(clock)
	}
	// "off" keeps the words and the ticking clock but drops every moving part —
	// including the throbber and the shade pulse, since "off" that still wiggles
	// isn't off. Same shrink order: full label, short label, bullet alone.
	if style == barOff {
		offRow := func(label string) string {
			return " " + cmpLabel.Render(strings.TrimSpace(ornBullet+" "+label)) + " " + cmpTime.Render(clock)
		}
		offFixed := lead + 2 + 1 + lipgloss.Width(clock) // bullet + its space, the gap, the clock
		switch {
		case width >= offFixed+longW:
			return offRow(long)
		case width >= offFixed+shortW:
			return offRow(short)
		default:
			return offRow("")
		}
	}
	// Richest layout that fits. A shortened label buys back a track before the
	// track is dropped outright: past the first second the words are redundant
	// (the transcript already logged "→ compacting context…") and the motion is
	// the part that says this is still alive.
	switch {
	case width-fixed-longW-1 >= 4: // the track absorbs the slack
		return row(long, width-fixed-longW-1)
	case width-fixed-shortW-1 >= 4:
		return row(short, width-fixed-shortW-1)
	case width >= fixed+longW:
		return row(long, 0)
	case width >= fixed+shortW:
		return row(short, 0)
	case width >= lead+waveW+lipgloss.Width(clock):
		return " " + wave + " " + cmpTime.Render(clock)
	default:
		return " " + wave
	}
}

// shadeWave is the 3-cell CP437 gradient pulse that leads the label — the ░▒▓█
// ramp marching in place, the same block vocabulary the banner deco uses. It
// steps at half the track's rate; at the full rate it reads as flicker rather
// than flow. Shared by every track style, so the row still has a heartbeat when
// the track itself is a calm one.
func shadeWave(phase int) string {
	var b strings.Builder
	for i := 0; i < 3; i++ {
		// Later cells lead the ramp, so the gradient reads as flowing rightward.
		b.WriteRune(shadeRamp[modPos(phase/2+i, len(shadeRamp))])
	}
	return cmpComet.Render(b.String())
}

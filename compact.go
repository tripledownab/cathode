// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ---- /compact: state, lifecycle, and the outcome line ----
//
// (The bar's layout is in compactbar.go; its track animations in
// compactstyle.go.)
//
// The CLI reports compaction as two events and nothing in between: a
// system/status "compacting" line, then a compact_result success/failed line
// (stream.go). There is no percentage, no token count, no step counter — so a
// filling bar would be inventing a number. This is an *indeterminate* bar: a
// comet sweeping a fixed track, plus the real elapsed seconds, which is the
// only honest progress signal we have. The real figures arrive afterwards on a
// system/compact_boundary (events.go:CompactMeta), which is what noteCompaction
// below turns into the outcome line.
//
// It animates off the existing spinner tick rather than arming a second one —
// armSpinnerIfNeeded keeps that tick running while compacting, not just while
// busy, so the bar still animates if the turn's busy flag has been cleared. The
// frame is derived from elapsed time instead of a counter, so nothing has to be
// incremented in the update loop.

// compact bar styles, rebuilt from the active palette by buildStyles (theme.go).
var (
	cmpLabel lipgloss.Style
	cmpComet lipgloss.Style
	cmpTrack lipgloss.Style
	cmpTime  lipgloss.Style
)

func buildCompactStyles() {
	cmpLabel = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	// Magenta comet on a gray track — same pairing as the scrollbar thumb/track,
	// so "this glyph is the moving one" reads the same way across the UI.
	cmpComet = lipgloss.NewStyle().Foreground(colMag).Bold(true)
	cmpTrack = lipgloss.NewStyle().Foreground(colGray)
	cmpTime = lipgloss.NewStyle().Foreground(colGray)
}

// compactFPS is how fast the comet steps. Matches the spinner tick rate closely
// enough that most frames advance exactly one cell.
const compactFPS = 10

// startCompacting/stopCompacting toggle the bar and resize the transcript, since
// the bar occupies a terminal row while it's up. Idempotent — a repeated
// "compacting" status (or a result arriving after an interrupt already cleared
// it) doesn't restart the clock or re-resize.
func (m *model) startCompacting() {
	if m.compacting() {
		return
	}
	m.compactAt = time.Now()
	m.compactMeta = nil // never carry a previous compaction's numbers into this one
	// Snapshot the context size now: it's what sizes the duration hint, and the
	// success status zeroes the gauge before the bar comes down.
	m.compactFrom = m.ctxTokens
	m.resizeViewport()
}

func (m *model) stopCompacting() {
	if !m.compacting() {
		return
	}
	m.compactAt = time.Time{}
	m.resizeViewport()
}

// compacting reports whether the progress bar is up.
func (m *model) compacting() bool { return !m.compactAt.IsZero() }

// compactDoneText is the fallback line when no compact_boundary showed up —
// an older CLI, or a compaction that ended without one.
const compactDoneText = "✓ context compacted"

// noteCompaction takes the real numbers off a compact_boundary. The boundary
// and the success status are independent events with no guaranteed order, so
// this handles both: if the plain line is already the last thing in the
// transcript, upgrade it in place; otherwise stash the numbers for the status
// handler to use. Either way exactly one line reports the compaction.
func (m *model) noteCompaction(meta CompactMeta) {
	// The exact post-compact context size, which is otherwise only inferable
	// from the next turn's usage — the success status zeroes the gauge as a
	// placeholder, so this replaces a guess with the real figure.
	if meta.PostTokens > 0 {
		m.observeCtx(meta.PostTokens)
	}
	if i := len(m.entries) - 1; i >= 0 && m.entries[i].kind == entInfo && m.entries[i].text == compactDoneText {
		m.entries[i].text = compactSummary(meta)
		m.rerender() // the cached render of that entry is now stale
		return
	}
	m.compactMeta = &meta
}

// compactSummary is the one-line outcome: how much context the compaction
// freed, how long it took, and (when reported) how many messages went into the
// summary. Every field is optional — the CLI omits some — so this degrades to
// however much it was actually told.
//
//	✓ compacted 23.0K → 1.9K in 33s
//	✓ auto-compacted 1.0M → 42.1K in 2m5s · 118 messages
func compactSummary(meta CompactMeta) string {
	verb := "compacted"
	if meta.Trigger == "auto" {
		// Worth calling out: an auto-compaction is one the user didn't ask for.
		verb = "auto-compacted"
	}
	s := "✓ " + verb
	switch {
	case meta.PreTokens > 0 && meta.PostTokens > 0:
		s += " " + abbreviateTokens(meta.PreTokens) + " → " + abbreviateTokens(meta.PostTokens)
	case meta.PreTokens > 0:
		s += " " + abbreviateTokens(meta.PreTokens)
	}
	if meta.DurationMS > 0 {
		s += " in " + shortDuration(time.Duration(meta.DurationMS)*time.Millisecond)
	}
	if meta.MessagesSummarized > 0 {
		s += fmt.Sprintf(" · %d messages", meta.MessagesSummarized)
	}
	if s == "✓ "+verb {
		return compactDoneText // nothing usable reported
	}
	return s
}

// shortDuration formats a compaction runtime: "95s" under a minute, "2m5s"
// over. Compactions run 1–5 minutes, so neither end needs sub-second detail.
func shortDuration(d time.Duration) string {
	secs := int(d.Round(time.Second).Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%ds", secs/60, secs%60)
}

// compactRows is how many terminal rows the bar occupies (0 or 1); folded into
// the viewport height math in resizeViewport.
func (m *model) compactRows() int {
	if m.compacting() {
		return 1
	}
	return 0
}

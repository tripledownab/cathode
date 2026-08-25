// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// The bar occupies a whole terminal row, so its rendered width must be exactly
// the window width at every size — one cell over and the frame wraps, shunting
// the status bar off screen.
func TestCompactBarWidth(t *testing.T) {
	for _, s := range barStyles {
		for _, w := range []int{20, 40, 60, 80, 120, 200} {
			for _, secs := range []int{0, 7, 90, 3600} {
				// "[█▒▒▒]" is the widest throbber (-spinner scan), so it's the one
				// most likely to push the row over.
				got := lipgloss.Width(compactBar(w, time.Duration(secs)*time.Second, "[█▒▒▒]", s.id, 500_000))
				if got > w {
					t.Errorf("%s width=%d elapsed=%ds: bar is %d cells wide, overflows", s.id, w, secs, got)
				}
			}
		}
	}
	// At a usable width an animated style should fill the row rather than leave
	// a ragged edge — the track absorbs the slack.
	for _, s := range barStyles {
		if s.id == barOff {
			continue // static: label + clock, no track to stretch
		}
		if got := lipgloss.Width(compactBar(80, time.Second, "/", s.id, 500_000)); got != 80 {
			t.Errorf("%s width=80: bar is %d cells, want a full-width row", s.id, got)
		}
	}
}

// Every track style must hand back exactly the cells it was given, at every
// phase — the track is what stretches the row to the window width, so a style
// that miscounts (e.g. len() over multi-byte block glyphs) wraps the frame.
func TestBarTrackWidths(t *testing.T) {
	for _, s := range barStyles {
		if s.id == barOff {
			continue
		}
		for _, w := range []int{1, 2, 5, 13, 40, 97} {
			for phase := 0; phase < 60; phase++ {
				if got := lipgloss.Width(barTrack(s.id, w, phase)); got != w {
					t.Fatalf("%s: barTrack(%d, phase %d) is %d cells", s.id, w, phase, got)
				}
			}
		}
	}
	// An id that isn't ours (hand-edited settings.json) still draws something.
	if got := lipgloss.Width(barTrack("nonsense", 30, 3)); got != 30 {
		t.Errorf("unknown style should fall back to a full-width track, got %d cells", got)
	}
}

// "off" means off: no throbber, no pulse, no track — just the words and the
// clock, which is the one part that isn't an animation.
func TestBarStyleOff(t *testing.T) {
	out := stripANSI(compactBar(100, 12*time.Second, "/", barOff, 500_000))
	for _, glyph := range []string{"░", "▒", "▓", "█", "─", "/"} {
		if strings.Contains(out, glyph) {
			t.Errorf("off style still animates: %q found in %q", glyph, out)
		}
	}
	if !strings.Contains(out, "12s") {
		t.Errorf("off style dropped the elapsed clock: %q", out)
	}
}

// A terminal too narrow for a track keeps the words — knowing *what* is running
// matters more than the animation.
func TestCompactBarNarrow(t *testing.T) {
	out := compactBar(24, 5*time.Second, "/", barComet, 500_000)
	if !strings.Contains(out, "5s") {
		t.Errorf("narrow bar dropped the elapsed clock: %q", out)
	}
	if lipgloss.Width(out) > 24 {
		t.Errorf("narrow bar overflows: %d cells", lipgloss.Width(out))
	}
}

// The comet sweeps right, turns around at the end, and never runs off either
// edge — the track always keeps its full cell count.
func TestCometTrackSweeps(t *testing.T) {
	const w = 40
	var positions []int
	for phase := 0; phase < 200; phase++ {
		out := cometTrack(w, phase)
		if got := lipgloss.Width(out); got != w {
			t.Fatalf("phase=%d: track is %d cells, want %d", phase, got, w)
		}
		positions = append(positions, strings.Index(stripANSI(out), "▒"))
	}
	if positions[0] != 0 {
		t.Errorf("phase 0 should start the comet at the left edge, got %d", positions[0])
	}
	// It must both advance and come back, else it's not bouncing.
	var sawForward, sawBack bool
	for i := 1; i < len(positions); i++ {
		switch {
		case positions[i] > positions[i-1]:
			sawForward = true
		case positions[i] < positions[i-1]:
			sawBack = true
		}
	}
	if !sawForward || !sawBack {
		t.Errorf("comet should bounce: forward=%v back=%v", sawForward, sawBack)
	}
}

// Lifecycle: the "compacting" status raises the bar and the transcript gives up
// a row for it; every way a compaction can end takes it back down again.
func TestCompactingLifecycle(t *testing.T) {
	newM := func() *model {
		m := &model{w: 100, h: 30, ready: true}
		m.resizeViewport()
		return m
	}

	base := newM()
	idleHeight := base.vp.Height

	m := newM()
	m.handleEvent(Envelope{Type: "system", Subtype: "status", Status: "compacting"})
	if !m.compacting() {
		t.Fatal("a compacting status should raise the bar")
	}
	if m.vp.Height != idleHeight-1 {
		t.Errorf("viewport height %d, want %d (one row yielded to the bar)", m.vp.Height, idleHeight-1)
	}
	// A repeated status must not restart the clock.
	at := m.compactAt
	m.handleEvent(Envelope{Type: "system", Subtype: "status", Status: "compacting"})
	if m.compactAt != at {
		t.Error("a repeated compacting status restarted the elapsed clock")
	}

	for _, end := range []struct {
		name string
		env  Envelope
	}{
		{"success", Envelope{Type: "system", Subtype: "status", CompactResult: "success"}},
		{"failed", Envelope{Type: "system", Subtype: "status", CompactResult: "failed", CompactError: "boom"}},
		{"turn result", Envelope{Type: "result"}},
	} {
		m := newM()
		m.handleEvent(Envelope{Type: "system", Subtype: "status", Status: "compacting"})
		m.handleEvent(end.env)
		if m.compacting() {
			t.Errorf("%s: bar should come down", end.name)
		}
		if m.vp.Height != idleHeight {
			t.Errorf("%s: viewport height %d, want the row back (%d)", end.name, m.vp.Height, idleHeight)
		}
	}
}

// boundaryEnv is the real shape observed on the wire (CLI 2.1.220), used so
// these tests exercise JSON decoding rather than a hand-built struct.
func boundaryEnv(t *testing.T, body string) Envelope {
	t.Helper()
	e, err := parseEnvelope([]byte(`{"type":"system","subtype":"compact_boundary","compact_metadata":` + body + `}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return e
}

// The compaction outcome is reported once, with the real numbers, whichever
// order the boundary and the success status arrive in. On the wire the boundary
// comes second, so that's the path that actually runs — but the CLI makes no
// ordering guarantee, so both are pinned.
func TestCompactBoundaryReporting(t *testing.T) {
	const meta = `{"trigger":"manual","pre_tokens":22987,"post_tokens":1943,"duration_ms":33334}`
	want := "✓ compacted 23.0K → 1.9K in 33s"

	infoLines := func(m *model) []string {
		var out []string
		for _, e := range m.entries {
			if e.kind == entInfo && strings.HasPrefix(e.text, "✓") {
				out = append(out, e.text)
			}
		}
		return out
	}

	// Observed order: status success, then boundary. The plain line is upgraded
	// in place rather than followed by a second one.
	after := &model{w: 100, h: 30, ready: true}
	after.resizeViewport()
	after.handleEvent(Envelope{Type: "system", Subtype: "status", Status: "compacting"})
	after.handleEvent(Envelope{Type: "system", Subtype: "status", CompactResult: "success"})
	after.handleEvent(boundaryEnv(t, meta))
	if got := infoLines(after); len(got) != 1 || got[0] != want {
		t.Errorf("boundary after status: got %q, want exactly [%q]", got, want)
	}

	// Reverse order: the numbers are stashed and used when the status lands.
	before := &model{w: 100, h: 30, ready: true}
	before.resizeViewport()
	before.handleEvent(Envelope{Type: "system", Subtype: "status", Status: "compacting"})
	before.handleEvent(boundaryEnv(t, meta))
	before.handleEvent(Envelope{Type: "system", Subtype: "status", CompactResult: "success"})
	if got := infoLines(before); len(got) != 1 || got[0] != want {
		t.Errorf("boundary before status: got %q, want exactly [%q]", got, want)
	}

	// post_tokens is the exact post-compact size; the status handler only zeroes
	// the gauge as a placeholder until it arrives. Both orders must land on the
	// real figure — zeroing after the boundary would throw it away.
	if after.ctxTokens != 1943 {
		t.Errorf("boundary after status: ctxTokens = %d, want 1943 from post_tokens", after.ctxTokens)
	}
	if before.ctxTokens != 1943 {
		t.Errorf("boundary before status: ctxTokens = %d, want 1943 (the status must not zero over it)", before.ctxTokens)
	}
	// Without post_tokens there's nothing better than the placeholder.
	noPost := &model{w: 100, h: 30, ready: true}
	noPost.resizeViewport()
	noPost.ctxTokens = 400_000
	noPost.handleEvent(Envelope{Type: "system", Subtype: "status", Status: "compacting"})
	noPost.handleEvent(boundaryEnv(t, `{"trigger":"manual","pre_tokens":410382,"duration_ms":91000}`))
	noPost.handleEvent(Envelope{Type: "system", Subtype: "status", CompactResult: "success"})
	if noPost.ctxTokens != 0 {
		t.Errorf("no post_tokens: ctxTokens = %d, want the 0 placeholder", noPost.ctxTokens)
	}

	// No boundary at all (older CLI, or a run that didn't emit one).
	bare := &model{w: 100, h: 30, ready: true}
	bare.resizeViewport()
	bare.handleEvent(Envelope{Type: "system", Subtype: "status", Status: "compacting"})
	bare.handleEvent(Envelope{Type: "system", Subtype: "status", CompactResult: "success"})
	if got := infoLines(bare); len(got) != 1 || got[0] != compactDoneText {
		t.Errorf("no boundary: got %q, want [%q]", got, compactDoneText)
	}

	// A previous compaction's numbers must not leak into the next one.
	after.handleEvent(Envelope{Type: "system", Subtype: "status", Status: "compacting"})
	if after.compactMeta != nil {
		t.Error("starting a compaction should clear stashed metadata")
	}
}

// Fields the CLI omits are dropped from the line, not rendered as zeros.
func TestCompactSummaryPartialMetadata(t *testing.T) {
	cases := []struct{ meta, want string }{
		{`{"trigger":"auto","pre_tokens":1000029,"post_tokens":42100,"duration_ms":125000,"messages_summarized":118}`,
			"✓ auto-compacted 1.0M → 42.1K in 2m5s · 118 messages"},
		{`{"trigger":"manual","pre_tokens":410382,"duration_ms":91000}`, // no post_tokens, as persisted records have it
			"✓ compacted 410.4K in 1m31s"},
		{`{"trigger":"manual"}`, // nothing usable
			compactDoneText},
	}
	for _, c := range cases {
		e := boundaryEnv(t, c.meta)
		if got := compactSummary(*e.CompactMeta); got != c.want {
			t.Errorf("compactSummary(%s)\n got %q\nwant %q", c.meta, got, c.want)
		}
	}
}

// The duration hint appears only once the wait is long enough to look like a
// hang, and is bucketed on context size — a flat "~2m" is wrong for a small
// session (a 23K compaction measured 33s).
func TestCompactDurationHint(t *testing.T) {
	early := stripANSI(compactBar(100, 5*time.Second, "/", barComet, 500_000))
	if strings.Contains(early, "typical") {
		t.Errorf("hint shown too early: %q", early)
	}
	big := stripANSI(compactBar(100, 30*time.Second, "/", barComet, 500_000))
	if !strings.Contains(big, "~2m typical") {
		t.Errorf("large context should hint ~2m: %q", big)
	}
	small := stripANSI(compactBar(100, 30*time.Second, "/", barComet, 23_000))
	if !strings.Contains(small, "~1m typical") {
		t.Errorf("small context should hint ~1m: %q", small)
	}
	// Unknown size takes the larger estimate rather than under-warning.
	if unknown := stripANSI(compactBar(100, 30*time.Second, "/", barComet, 0)); !strings.Contains(unknown, "~2m") {
		t.Errorf("unknown context size should hint ~2m: %q", unknown)
	}
	// Narrow terminals keep the animation and drop the note.
	narrow := stripANSI(compactBar(46, 30*time.Second, "/", barComet, 500_000))
	if strings.Contains(narrow, "typical") {
		t.Errorf("narrow bar should drop the hint, not the track: %q", narrow)
	}
}

// Compaction keeps the spinner tick alive even once busy has been cleared —
// that tick is the only thing producing the frames the bar animates on.
func TestCompactArmsSpinner(t *testing.T) {
	m := &model{}
	if c := m.armSpinnerIfNeeded(); c != nil || m.spinning {
		t.Fatal("idle and not compacting: no tick should be armed")
	}
	m.startCompacting()
	if c := m.armSpinnerIfNeeded(); c == nil || !m.spinning {
		t.Fatal("compacting should arm the tick even when not busy")
	}
}

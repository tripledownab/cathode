// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import "strings"

// ---- /compact bar animation styles ----
//
// The track is the animated middle of the compact progress bar (compact.go).
// Every renderer here must return exactly `width` cells at every phase — the
// bar is a full terminal row, and one cell over wraps the frame. Add a style by
// adding an id, a barStyles row, and a case in barTrack.

const (
	barComet  = "comet"
	barBarber = "barber"
	barPulse  = "pulse"
	barScan   = "scan"
	barOff    = "off"
)

// barStyleDef is one row in the /settings compact-bar picker. The desc carries
// a plain-text sample of the style so the menu previews it — the real thing
// only ever shows up mid-compaction, which is a bad time to go shopping.
type barStyleDef struct{ id, label, desc string }

var barStyles = []barStyleDef{
	{barComet, "comet", "tapered block sweeping end to end   ░░░▒▓██▓▒░░"},
	{barBarber, "barber pole", "CP437 gradient marching rightward   ░▒▓█▓▒░▒▓█"},
	{barPulse, "pulse", "the whole track breathing as one    ▓▓▓▓▓▓▓▓▓▓▓"},
	{barScan, "scanner", "single block bouncing a thin rail   ──█────────"},
	{barOff, "off (static)", "no track — just the label and the elapsed clock"},
}

func barLabel(id string) string {
	for _, s := range barStyles {
		if s.id == id {
			return s.label
		}
	}
	return id
}

// validBarStyle reports whether id names a real style — used by /bar to reject
// a typo instead of silently persisting one and falling back to the comet.
func validBarStyle(id string) bool {
	for _, s := range barStyles {
		if s.id == id {
			return true
		}
	}
	return false
}

func barItems() []pickerItem {
	items := make([]pickerItem, 0, len(barStyles))
	for _, s := range barStyles {
		items = append(items, pickerItem{id: s.id, title: s.label, subtitle: s.desc})
	}
	return items
}

// commitBar persists the compact-bar animation. Chrome only — nothing in the
// transcript re-renders, so the next frame just draws the new style.
func (m *model) commitBar(id string) {
	m.settings.Bar = id
	saveSettings(m.settings)
	m.add(entInfo, "→ compact bar: "+barLabel(id))
}

// barTrack draws one frame of the chosen style. Unknown ids fall back to the
// comet so a hand-edited settings.json can't blank the bar.
func barTrack(style string, width, phase int) string {
	if width <= 0 {
		return ""
	}
	switch style {
	case barBarber:
		return barberTrack(width, phase)
	case barPulse:
		return pulseTrack(width, phase)
	case barScan:
		return scanTrack(width, phase)
	default:
		return cometTrack(width, phase)
	}
}

// shadeRamp is the CP437 density ladder, up and back down, so anything cycling
// through it breathes rather than snapping from full block to empty.
var shadeRamp = []rune("░▒▓█▓▒")

// cometTrack draws a tapered comet at the phase's position on a ░ track,
// reversing direction at each end. The comet is symmetric, so it looks the same
// going both ways.
func cometTrack(width, phase int) string {
	// Runes, not bytes: every glyph here is a 3-byte block, so len() on the
	// string would undercount the track by two thirds.
	comet := []rune(cometGlyphs(width))
	span := width - len(comet)
	if span <= 0 {
		return cmpComet.Render(strings.Repeat("█", width))
	}
	pos := bouncePos(phase, span)
	return cmpTrack.Render(strings.Repeat("░", pos)) +
		cmpComet.Render(string(comet)) +
		cmpTrack.Render(strings.Repeat("░", span-pos))
}

// cometGlyphs is the comet itself, sized to the track: a bright core fading out
// both ways. Shrinks on narrow terminals so there's always track left to sweep.
func cometGlyphs(width int) string {
	switch {
	case width >= 14:
		return "▒▓██▓▒"
	case width >= 8:
		return "▓█▓"
	default:
		return "█"
	}
}

// Rate divisors against the bar's phase (compactFPS steps/sec). The two styles
// that animate the *whole* track are deliberately slower than the two that move
// a single mark across it: a full-width texture changing 10×/sec reads as a
// strobe, not as progress.
const (
	barberDiv = 2 // ~5 cells/sec of drift
	pulseDiv  = 6 // a full ░▒▓█▓▒ breath every ~3.5s
)

// barberTrack fills the whole track with the shade ramp, shifted along the
// phase so the gradient marches rightward. One styled run for the lot — the
// density ladder carries the contrast, so it doesn't need per-cell color.
func barberTrack(width, phase int) string {
	var b strings.Builder
	for i := 0; i < width; i++ {
		// i-phase (not i+phase): subtracting moves the pattern with the phase
		// instead of against it.
		b.WriteRune(shadeRamp[modPos(i-phase/barberDiv, len(shadeRamp))])
	}
	return cmpComet.Render(b.String())
}

// pulseTrack cycles the entire track through the ramp as one block — the
// calmest of the styles, and the cheapest (a single repeated rune).
func pulseTrack(width, phase int) string {
	return cmpComet.Render(strings.Repeat(string(shadeRamp[modPos(phase/pulseDiv, len(shadeRamp))]), width))
}

// scanTrack bounces a single hard block along a thin rail — the Knight Rider
// read, with no taper to soften it.
func scanTrack(width, phase int) string {
	span := width - 1
	if span <= 0 {
		return cmpComet.Render("█")
	}
	pos := bouncePos(phase, span)
	return cmpTrack.Render(strings.Repeat("─", pos)) +
		cmpComet.Render("█") +
		cmpTrack.Render(strings.Repeat("─", span-pos))
}

// bouncePos maps a monotonic phase onto [0, span] as a triangle wave: walk
// right for span steps, then back left.
func bouncePos(phase, span int) int {
	if span <= 0 {
		return 0
	}
	pos := modPos(phase, 2*span)
	if pos > span {
		pos = 2*span - pos
	}
	return pos
}

// modPos is a modulo that stays non-negative for negative operands (Go's %
// keeps the dividend's sign, which would index out of range).
func modPos(a, n int) int { return ((a % n) + n) % n }

// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

// Jump-to-prompt (shift+↑ / shift+↓).
//
// Stepping back through your own turns is a scroll, not a mode: the target
// prompt's first line becomes the viewport's top line, and that's the whole
// interaction. The anchor is *derived* from vp.YOffset rather than stored, so
// jumping composes with every other way the transcript moves — wheel, PageUp,
// auto-follow — and there's no cursor to invalidate when entries are appended
// or /clear drops them all.
//
// The entry → line mapping is m.entryLine, maintained by appendEntry
// (render.go) as the transcript is rendered.

// jumpPrompt scrolls to the nearest user entry above (delta < 0) or below
// (delta > 0) the current top line. Stepping past the newest prompt returns to
// the bottom and re-arms auto-follow — that's how you get back to live output.
// A press with nothing to move to is a no-op (still consumed, so it never
// leaks into the prompt).
func (m *model) jumpPrompt(delta int) {
	// entryLine only covers what's been rendered (== renderedCount), which lags
	// entries by at most the tail rebuild() is about to add.
	n := len(m.entryLine)
	if n > len(m.entries) {
		n = len(m.entries)
	}
	if !m.ready || n == 0 {
		return
	}
	off := m.vp.YOffset
	if delta < 0 {
		// Strictly above the top line, so repeated presses keep walking back
		// instead of re-selecting the prompt already parked at the top.
		for i := n - 1; i >= 0; i-- {
			if m.entries[i].kind == entUser && m.entryLine[i] < off {
				m.scrollToLine(m.entryLine[i])
				return
			}
		}
		return
	}
	for i := 0; i < n; i++ {
		if m.entries[i].kind == entUser && m.entryLine[i] > off {
			m.scrollToLine(m.entryLine[i])
			return
		}
	}
	// Nothing below: we're at the last prompt already, so rejoin the stream.
	m.follow = true
	m.vp.GotoBottom()
}

// scrollToLine parks a content line at the top of the viewport. SetYOffset
// clamps within the last screenful, so a prompt near the end lands as close to
// the top as it can — and if that clamp puts us at the bottom, auto-follow
// re-arms rather than leaving the transcript silently frozen.
func (m *model) scrollToLine(line int) {
	m.vp.SetYOffset(line)
	m.follow = m.vp.AtBottom()
}

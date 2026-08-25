// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// Scroll / auto-follow.
//
// The transcript normally "sticks" to the bottom so streaming output stays in
// view. m.follow tracks whether that stickiness is active: rebuild() jumps to
// the latest line only while it's set. The user scrolling up (mouse wheel or
// PageUp) clears it, so reading back through history is never yanked down by
// new content; scrolling back to the bottom re-arms it, and typing into the
// prompt always snaps to the latest.

// newTranscriptViewport builds the transcript viewport with its built-in scroll
// keys stripped. The default KeyMap binds space/f/b/u/d/j/k, which collide with
// typing into the always-focused prompt. Scrolling is driven by the mouse wheel
// and explicit PageUp/PageDown handling (keys.go) instead.
func newTranscriptViewport(w, h int) viewport.Model {
	vp := viewport.New(w, h)
	vp.KeyMap = viewport.KeyMap{}
	return vp
}

// setMouseCapture switches wheel capture on/off, notes it in the transcript, and
// returns the Bubble Tea command that reconfigures the terminal. Shared by the
// /mouse command and the shift+wheel gesture (see update.go) so the two entry
// points can't drift.
func (m *model) setMouseCapture(on bool) tea.Cmd {
	m.mouse = on
	if on {
		m.add(entInfo, "→ mouse: ON — wheel scrolls the transcript")
		return tea.EnableMouseCellMotion
	}
	m.add(entInfo, "→ mouse: OFF — drag to select/copy · wheel or ↑/↓ scrolls · ctrl+↑/↓ history")
	return tea.DisableMouse
}

// syncScroll reconciles m.follow with what the user just did. Called at the tail
// of Update, after the raw msg has been handed to the input and viewport.
func (m *model) syncScroll(msg tea.Msg, prevInput string) {
	switch msg.(type) {
	case tea.MouseMsg:
		// A wheel scroll settled the viewport somewhere; follow iff that's the
		// bottom. Non-scroll mouse events leave the offset untouched, so this
		// just re-reads the same answer.
		m.follow = m.vp.AtBottom()
	case tea.KeyMsg:
		// A keystroke that changed the prompt means the user is composing a
		// reply — pull them back to the latest output.
		if m.input.Value() != prevInput {
			m.follow = true
			m.vp.GotoBottom()
		}
	}
}

// ---- Auto-hiding scrollbar ----
//
// The transcript's right-edge scrollbar (bbsScrollbar, joined in view.go) is a
// column of │/┃ box glyphs. Terminal drag-select grabs whatever is rendered in
// that column, so copying the transcript pulls a pipe onto the end of every
// line. To keep copies clean we show the glyphs only briefly after a user scroll
// and otherwise render the column as spaces — the width stays reserved either
// way, so nothing reflows as it appears/disappears.

// scrollbarHideAfter is how long the scrollbar stays visible after a scroll.
const scrollbarHideAfter = 1500 * time.Millisecond

// scrollHideMsg wakes the loop when the visibility window lapses so refreshBody
// re-renders the column blank — the scroll offset didn't change, so nothing else
// would invalidate the memoized body on its own.
type scrollHideMsg struct{}

func scrollHideTick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return scrollHideMsg{} })
}

// scrollbarVisible reports whether the glyphs should show right now. Starts
// false (the zero scrollShownAt is far in the past), so a fresh session and any
// untouched transcript copy clean.
func (m *model) scrollbarVisible() bool {
	return time.Since(m.scrollShownAt) < scrollbarHideAfter
}

// pokeScrollbar marks the scrollbar visible for scrollbarHideAfter and arms the
// single hide tick if none is running. Called from the offset-change checks in
// Update whenever a user gesture (wheel, PageUp/Down, ↑/↓ in select mode) moves
// the viewport — never on streaming auto-follow, so it doesn't flash while
// Claude types.
func (m *model) pokeScrollbar() tea.Cmd {
	m.scrollShownAt = time.Now()
	if m.scrollTicking {
		return nil
	}
	m.scrollTicking = true
	return scrollHideTick(scrollbarHideAfter)
}

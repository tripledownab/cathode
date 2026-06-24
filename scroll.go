package main

import (
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

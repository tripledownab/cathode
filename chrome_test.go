// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// The auto-hiding scrollbar draws its │/┃ glyphs only while "visible"; hidden,
// it renders an equal-height column of blank spaces (same width, no reflow) so a
// drag-select copy of the transcript picks up no pipes. See scroll.go.
func TestScrollbarAutoHideRendering(t *testing.T) {
	const h, content, view = 6, 40, 6 // content > view → a real scrollbar
	shown := bbsScrollbar(h, content, view, 0, true)
	hidden := bbsScrollbar(h, content, view, 0, false)

	if lineCount(shown) != h || lineCount(hidden) != h {
		t.Fatalf("height changed between states: shown=%d hidden=%d want=%d",
			lineCount(shown), lineCount(hidden), h)
	}
	if !strings.ContainsAny(shown, "│┃") {
		t.Errorf("visible scrollbar should draw glyphs, got %q", shown)
	}
	if strings.Trim(hidden, " \n") != "" {
		t.Errorf("hidden scrollbar must be blank spaces only (else copy corrupts), got %q", hidden)
	}
}

// A user scroll surfaces the scrollbar (glyphs land in the body); once the
// visibility window lapses it hides again, so the transcript copies clean.
func TestScrollbarPokeThenLapse(t *testing.T) {
	m := newModel(&Engine{}, "ask", nil, "bar", "")
	m.splash = false
	for i := 0; i < 60; i++ { // overflow the viewport so a scrollbar exists
		m.add(entInfo, fmt.Sprintf("transcript line %d", i))
	}
	if m.scrollbarVisible() {
		t.Fatal("scrollbar should start hidden")
	}
	if strings.Contains(m.renderBody(), "┃") {
		t.Fatal("hidden scrollbar leaked a thumb glyph into the body")
	}
	m.pokeScrollbar()
	if !m.scrollbarVisible() {
		t.Fatal("pokeScrollbar should make the scrollbar visible")
	}
	if !strings.Contains(m.renderBody(), "┃") {
		t.Fatal("visible scrollbar should draw a thumb glyph in the body")
	}
	m.scrollShownAt = time.Now().Add(-scrollbarHideAfter - time.Second)
	if m.scrollbarVisible() {
		t.Fatal("scrollbar should hide once the window lapses")
	}
	if strings.Contains(m.renderBody(), "┃") {
		t.Fatal("lapsed scrollbar should render blank again")
	}
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// The composed frame must fit the terminal height at every window size that can
// still hold the chrome — the transcript viewport absorbs the slack and never
// pushes the total past the terminal.
func TestFrameFitsHeight(t *testing.T) {
	for _, h := range []int{14, 16, 20, 30, 40} {
		m := newModel(&Engine{}, "ask", nil, "bar", "")
		m.w, m.h = 80, h
		m.setPromptWidth(m.w - 4)
		m.resizeViewport()
		m.makeRenderer()
		for i := 0; i < 80; i++ {
			m.add(entClaude, "streamed assistant output line")
		}
		m.busy = true
		m.resizeViewport()
		m.refreshBody()
		got := lineCount(m.renderBackground())
		if got > h {
			t.Errorf("h=%d: frame %d lines > terminal %d", h, got, h)
		}
		if m.vp.Height < 1 {
			t.Errorf("h=%d: viewport starved to %d rows", h, m.vp.Height)
		}
	}
}

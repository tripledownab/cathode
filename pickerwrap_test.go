// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// longDescription is wider than any box the picker draws, so a one-line row
// has to cut it. Its last word is the part a cut loses.
const longDescription = "Keep the current schema and add a nullable column, " +
	"backfill it in batches overnight, then flip the default once every row " +
	"has a value and the old readers are gone. ENDMARK"

func wrapPicker(cursor, w, h int) *picker {
	items := []pickerItem{
		{id: "a", title: "Migrate in place", subtitle: longDescription},
		{id: "b", title: "New table", subtitle: longDescription},
		{id: "c", title: "Do nothing", subtitle: "short"},
	}
	p := newPicker("question", "Which migration?", items, w, h)
	p.wrap = true
	p.cursor = cursor
	return p
}

// A wrapped description is shown whole, for the selected row and the others.
func TestWrapPickerShowsTheWholeDescription(t *testing.T) {
	for cursor := 0; cursor < 3; cursor++ {
		body := stripANSI(wrapPicker(cursor, 64, 40).View())
		if got := strings.Count(body, "ENDMARK"); got != 2 {
			t.Errorf("cursor %d: the description end is shown %d times, want 2:\n%s",
				cursor, got, body)
		}
	}
}

// No line may wrap a second time inside the box. The box pads every line to
// one width, so only a line count shows the break (see the two-line test).
func TestWrapPickerRowsDoNotWrap(t *testing.T) {
	p := wrapPicker(1, 64, 40)
	inner := 64 - 4 - 2 - 1 - approveBar.GetHorizontalFrameSize()
	want := 2 + 2 + 1 // border, title and filter input, footer
	for _, idx := range p.filtered {
		want += len(p.wrapItemLines(idx, inner))
	}
	if got := len(strings.Split(stripANSI(p.View()), "\n")); got != want {
		t.Errorf("rendered %d lines, want %d — a row wrapped:\n%s",
			got, want, stripANSI(p.View()))
	}
}

// On a short terminal the window keeps the cursor item in view and draws no
// more rows than fit.
func TestWrapPickerWindowFitsTheTerminal(t *testing.T) {
	const h = 16
	for cursor := 0; cursor < 3; cursor++ {
		p := wrapPicker(cursor, 64, h)
		body := stripANSI(p.View())
		if got := len(strings.Split(body, "\n")); got > h {
			t.Errorf("cursor %d: %d lines on a %d-row terminal:\n%s", cursor, got, h, body)
		}
		title := p.items[p.filtered[cursor]].title
		if !strings.Contains(body, title) {
			t.Errorf("cursor %d: the selected item %q is not drawn:\n%s", cursor, title, body)
		}
	}
}

func TestWrapWindow(t *testing.T) {
	cases := []struct {
		heights            []int
		cursor, max        int
		wantStart, wantEnd int
	}{
		{[]int{3, 3, 3}, 0, 10, 0, 3}, // everything fits
		{[]int{3, 3, 3}, 0, 6, 0, 2},  // fills forward from the top
		{[]int{3, 3, 3}, 2, 6, 1, 3},  // walks back from the cursor
		{[]int{9, 1}, 0, 4, 0, 1},     // one tall item still shows
	}
	for _, c := range cases {
		s, e := wrapWindow(c.heights, c.cursor, c.max)
		if s != c.wantStart || e != c.wantEnd {
			t.Errorf("wrapWindow(%v, %d, %d) = %d,%d, want %d,%d",
				c.heights, c.cursor, c.max, s, e, c.wantStart, c.wantEnd)
		}
	}
}

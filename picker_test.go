// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// A styled row must not wrap. The lightbar style adds a column of padding on
// each side, so padding the content to the full list width renders two columns
// too wide; the row wraps, and the scrollbar gutter then interleaves with the
// text instead of sitting beside it.
//
// Counting lines, not widths: the surrounding box pads every line to the same
// width whatever happens inside it, so a width check passes even while the
// layout is broken.
func TestTwoLinePickerRowsDoNotWrap(t *testing.T) {
	const items, h = 6, 24
	rows := make([]pickerItem, items)
	for i := range rows {
		rows[i] = pickerItem{
			id:       string(rune('a' + i)),
			title:    strings.Repeat("title ", 6),
			subtitle: strings.Repeat("detail ", 6),
		}
	}
	p := newPicker("sessions", "RESUME SESSION", rows, 64, h)
	p.twoLine = true
	p.cursor = 1 // the selected row is the styled one, and the one that wrapped

	got := len(strings.Split(stripANSI(p.View()), "\n"))
	// 2 border + title + filter input + 2 per item + footer.
	want := 2 + 2 + items*2 + 1
	if got != want {
		t.Errorf("rendered %d lines, want %d — a row wrapped:\n%s",
			got, want, stripANSI(p.View()))
	}
}

// Half as many items fit when each takes two rows, or the list renders past the
// bottom of the box.
func TestTwoLinePickerShowsHalfAsManyItems(t *testing.T) {
	items := make([]pickerItem, 40)
	for i := range items {
		items[i] = pickerItem{id: string(rune('a' + i%26)), title: "t", subtitle: "s"}
	}
	count := func(two bool) int {
		p := newPicker("sessions", "T", items, 64, 40)
		p.twoLine = two
		body := stripANSI(p.View())
		return strings.Count(body, "\n")
	}
	if one, two := count(false), count(true); one != two {
		t.Errorf("box height changed with the row style: %d vs %d lines", one, two)
	}
}

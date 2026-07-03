package main

import (
	"strings"
	"unicode"

	rw "github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// promptVisualRows reports how many terminal rows the prompt textarea needs to
// show value with nothing hidden: hard lines plus soft-wrap. The bubbles
// textarea soft-wraps internally but exposes only LineCount() (hard lines), so
// sizing by that left a long line in a narrow window one row tall, scrolled to
// the cursor — you typed blind. Its wrap cache is private, so this counts rows
// with a port of the same algorithm.
func promptVisualRows(value string, width int) int {
	if width < 1 {
		width = 1
	}
	rows := 0
	for _, line := range strings.Split(value, "\n") {
		rows += wrappedRowCount([]rune(line), width)
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

// wrappedRowCount is the bubbles textarea's wrap() (v0.20.0, textarea.go)
// reduced to counting rows. Kept structurally identical — word buffer, space
// run, double-width rune handling, and the trailing >=width spill row — so the
// count matches what the widget actually renders.
func wrappedRowCount(runes []rune, width int) int {
	var (
		lines  = [][]rune{{}}
		word   = []rune{}
		row    int
		spaces int
	)
	for _, r := range runes {
		if unicode.IsSpace(r) {
			spaces++
		} else {
			word = append(word, r)
		}
		if spaces > 0 {
			if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces > width {
				row++
				lines = append(lines, []rune{})
			}
			lines[row] = append(lines[row], word...)
			lines[row] = append(lines[row], []rune(strings.Repeat(" ", spaces))...)
			spaces = 0
			word = nil
		} else {
			lastCharLen := rw.RuneWidth(word[len(word)-1])
			if uniseg.StringWidth(string(word))+lastCharLen > width {
				if len(lines[row]) > 0 {
					row++
					lines = append(lines, []rune{})
				}
				lines[row] = append(lines[row], word...)
				word = nil
			}
		}
	}
	if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces >= width {
		lines = append(lines, []rune{})
	}
	return len(lines)
}

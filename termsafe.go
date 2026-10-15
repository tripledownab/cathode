// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"unicode/utf8"
)

// termSafe removes everything from a rendered entry that a terminal would act
// on instead of drawing, except colour (SGR) sequences. Text, newlines and tabs
// stay, and so does every printable or zero-width rune, combining marks
// included.
//
// Tool output reaches the transcript as the program wrote it, and a program
// writing to a terminal moves the cursor: an interactive prompt clears and
// redraws its line, a progress bar returns to column one. Those sequences
// reach the real terminal inside a frame, where a cursor-up or an erase-line
// overwrites rows the renderer has already drawn. A lone \r is dropped rather
// than honoured, because the entry is already rendered and returning to the
// start of a line would also overwrite the card's border.
//
// Only ESC starts a sequence here, as in a UTF-8 terminal. A byte left over
// from a cut-off rune becomes U+FFFD, which is what the terminal would draw,
// and never swallows the text after it.
//
// It runs before linkify, so the hyperlinks linkify adds survive.
func termSafe(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\x1b':
			n := escLen(s[i:])
			if seq := s[i : i+n]; isSGR(seq) {
				b.WriteString(seq)
			}
			i += n
		case c == '\n' || c == '\t':
			b.WriteByte(c)
			i++
		case c < 0x20 || c == 0x7f:
			i++
		case c < utf8.RuneSelf:
			b.WriteByte(c)
			i++
		default:
			r, n := utf8.DecodeRuneInString(s[i:])
			switch {
			case r == utf8.RuneError && n == 1:
				b.WriteRune(utf8.RuneError)
			case r >= 0x80 && r <= 0x9f: // C1 controls
			default:
				b.WriteString(s[i : i+n])
			}
			i += n
		}
	}
	return b.String()
}

// escLen returns how many bytes of s, which starts with ESC, belong to the
// escape sequence. A sequence that is cut off or malformed ends at the first
// byte that cannot belong to it. A string sequence with no terminator ends at
// the line, so it never swallows the lines after it.
func escLen(s string) int {
	if len(s) < 2 {
		return len(s)
	}
	switch s[1] {
	case '[': // CSI: parameter and intermediate bytes, then one final byte
		i := 2
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x3f {
			i++
		}
		if i < len(s) && s[i] >= 0x40 && s[i] <= 0x7e {
			i++
		}
		return i
	case ']', 'P', '_', '^', 'X': // a string, ended by BEL or ESC \
		for i := 2; i < len(s); i++ {
			switch s[i] {
			case '\a':
				return i + 1
			case '\x1b':
				if i+1 < len(s) && s[i+1] == '\\' {
					return i + 2
				}
				return i
			case '\n':
				return i
			}
		}
		return len(s)
	}
	// Any other escape: intermediate bytes, then one final byte.
	i := 1
	for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
		i++
	}
	if i < len(s) && s[i] >= 0x30 && s[i] <= 0x7e {
		i++
	}
	return i
}

// isSGR reports whether seq is a colour or style sequence: a CSI with only
// numeric parameters, ending in m.
func isSGR(seq string) bool {
	params, ok := strings.CutPrefix(seq, "\x1b[")
	if !ok || !strings.HasSuffix(params, "m") {
		return false
	}
	return strings.Trim(params[:len(params)-1], "0123456789;:") == ""
}

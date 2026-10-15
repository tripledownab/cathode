// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestTermSafeKeepsTextAndColourOnly(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"interactive prompt", "\x1b[?25l\x1b[2K\x1b[1G\x1b[36m?\x1b[39m pick one", "\x1b[36m?\x1b[39m pick one"},
		{"cursor up and erase", "done\x1b[1A\x1b[2K  OK", "done  OK"},
		{"progress return", "10%\r20%\r\n", "10%20%\n"},
		{"raw C1 byte", "\x9b31mtext", "\ufffd31mtext"},
		{"C1 as UTF-8", "a\u009b2Jb", "a2Jb"},
		{"cursor save and charset", "\x1b7\x1b(0q\x1b8", "q"},
		{"unterminated title ends at the line", "\x1b]0;title\nnext", "\nnext"},
		{"device string", "\x1bPq#0;2;0;0;0\x1b\\after", "after"},
		{"cut-off emoji", "\xf0\x9f\x91xyz\nnext", "\ufffd\ufffd\ufffdxyz\nnext"},
		{"combining marks", "Cafe\u0301 1\ufe0f\u20e3 a\u200db", "Cafe\u0301 1\ufe0f\u20e3 a\u200db"},
		{"title and bell", "\x1b]0;window\x07build\a", "build"},
		{"key mode ending in m", "\x1b[>4;2mkeys", "keys"},
		{"colour with subparams", "\x1b[38:5:208mx\x1b[0m", "\x1b[38:5:208mx\x1b[0m"},
		{"wide and box runes", "│ 字 ✅ ě\tend", "│ 字 ✅ ě\tend"},
	} {
		if got := termSafe(c.in); got != c.want {
			t.Errorf("%s: termSafe(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// sgr matches the one escape sequence the transcript may carry. The check
// below does not ask termSafe, so a filter that lets too much through fails it.
var sgr = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// A tool result is the program's terminal output verbatim. Every entry goes
// through displayEntry, so nothing that moves the cursor reaches the frame.
func TestToolOutputCannotMoveTheCursor(t *testing.T) {
	m := fullTranscript(t)
	m.entries = append(m.entries, entry{kind: entToolResult, toolName: "Bash",
		toolResult: "\x1b[?25l\x1b[2K\x1b[1Gstep one\x1b[1A\x1b[2K\rstep two"})
	m.add(entInfo, "END-MARKER")
	for _, l := range strings.Split(m.content.String(), "\n") {
		if strings.ContainsFunc(sgr.ReplaceAllString(l, ""), func(r rune) bool {
			return r < 0x20 && r != '\t' || r == 0x7f || r >= 0x80 && r <= 0x9f
		}) {
			t.Fatalf("a transcript line still carries control sequences: %q", l)
		}
	}
	if !lastLineShows(m) {
		t.Fatal("the last line is not in view")
	}
}

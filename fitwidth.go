// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// displayEntry renders e as the viewport will hold it. Every entry goes
// through here: termSafe removes what would move the terminal's cursor,
// linkify adds hyperlinks, and fitWidth breaks lines the viewport would cut.
func (m *model) displayEntry(e entry) string {
	return fitWidth(linkify(termSafe(m.renderEntry(e))), m.vp.Width)
}

// fitWidth breaks every line of a rendered entry that is wider than w.
//
// The viewport does not tolerate a wide line. Its View pads and wraps the
// visible lines to its width and then cuts them to its height, so each extra
// row a wide line wraps into pushes one row off the bottom of the transcript.
// That bottom row is the newest output, and the cut is permanent: scrolling
// to the end only shows the same wrapped frame. Glamour wraps prose but never
// breaks a token, so a long path, URL, inline code span or code-block line
// reaches the viewport intact. A reply with a few of those lost its last lines.
//
// Breaking the lines here also keeps the line count in appendEntry exact,
// which jump.go depends on.
//
// Tabs are expanded first. ansi.StringWidth counts a tab as no cells, but the
// viewport's lipgloss render expands it, so a tabbed line measured as fitting
// still overflowed.
func fitWidth(s string, w int) string {
	if w < 1 {
		return s
	}
	s = expandTabs(s, tabWidth)
	wide := false
	for _, line := range strings.Split(s, "\n") {
		if ansi.StringWidth(line) > w {
			wide = true
			break
		}
	}
	if !wide {
		return s
	}
	return closeBrokenLines(ansi.Hardwrap(s, w, true))
}

// closeBrokenLines closes a hyperlink that a break left open at the end of a line
// and opens it again at the start of the next one. It also resets the style at
// the end of every line, because a break inside a styled token leaves its
// colour open. Without both, the scrollbar cell joined after the line takes
// the link and the colour.
func closeBrokenLines(s string) string {
	lines := strings.Split(s, "\n")
	target := ""
	for i, line := range lines {
		if target != "" {
			line = oscOpen + target + oscClose + line
		}
		target = openLink(line)
		if target != "" {
			line += oscOpen + oscClose
		}
		lines[i] = line + ansi.ResetStyle
	}
	return strings.Join(lines, "\n")
}

// openLink returns the target of the hyperlink still open at the end of line,
// or "" when none is. linkify ends every link with an empty-target open.
func openLink(line string) string {
	i := strings.LastIndex(line, oscOpen)
	if i < 0 {
		return ""
	}
	rest := line[i+len(oscOpen):]
	end := strings.Index(rest, oscClose)
	if end <= 0 {
		return ""
	}
	return rest[:end]
}

// tabWidth is the tab stop lipgloss expands to by default. The transcript
// keeps it so a reply looks as it did before tabs were expanded here, and so a
// leading tab in markdown still opens an indented code block.
const tabWidth = 4

func expandTabs(s string, n int) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", n))
}

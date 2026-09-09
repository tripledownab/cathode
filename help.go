// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The help modal and the /help transcript text. Both project the same command
// table (commandlist.go), so a new command documents itself.

// helpModalView is the boxed, centered version of the help text. Rendered by
// View() through lipgloss.Place so it looks like a floating modal.
func helpModalView(termW, termH int, backend string) string {
	w := termW - 8
	if w < 48 {
		w = 48
	}
	if w > 78 {
		w = 78
	}
	body := dTitle.Render(" HELP ") + "\n" + helpText(backend) + "\n" +
		cDim.Render("  [esc / ?] close")
	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colCyan).
		Padding(0, 1).
		Width(w)
	return box.Render(body)
}

// helpText is what /help prints into the transcript.
func helpText(backend string) string {
	cmds := slashCommands()
	sort.SliceStable(cmds, func(a, b int) bool { return cmds[a].name < cmds[b].name })
	var b strings.Builder
	b.WriteString("keybindings:\n")
	b.WriteString("  enter         send  ·  alt+enter / ctrl+j / \\↵  insert a line break\n")
	// Only claude injects the file's contents for an @path; codex receives the
	// text and has to read the file itself. Verified by probing both. Saying so
	// matters because the two look identical while typing.
	if backend == backendCodex {
		b.WriteString("  @             inline file picker — inserts @path (codex reads the file itself)\n")
	} else {
		b.WriteString("  @             inline file picker — inserts @path (claude expands it to file contents)\n")
	}
	b.WriteString("  shift+tab     cycle mode (plan → ask → build)\n")
	b.WriteString("  ctrl+r        resume a session\n")
	b.WriteString("  ctrl+t        slash command palette\n")
	b.WriteString("  ctrl+g        toggle the info sidebar (or /sidebar)\n")
	b.WriteString("  ?             open this help modal\n")
	b.WriteString("  ↑ / ↓         history · cursor between lines (multi-line) · scroll (mouse off)\n")
	b.WriteString("  ctrl+↑ / ↓    prompt history (always)\n")
	b.WriteString("  shift+↑ / ↓   jump to your previous / next prompt in the transcript\n")
	b.WriteString("  shift+scroll  drop into select mode (terminals that forward it; /mouse returns)\n")
	b.WriteString("  esc           interrupt the running turn (or quit when idle)\n")
	b.WriteString("  ctrl+c        clear the prompt · interrupt the turn · again to quit\n")
	b.WriteString("commands:\n")
	for _, c := range cmds {
		if !c.availableOn(backend) {
			continue
		}
		b.WriteString(fmt.Sprintf("  /%-10s %s\n", c.name, c.desc))
	}
	b.WriteString("  any other /command is forwarded to " + agentName(backend) + " (custom & plugin commands)\n")
	return strings.TrimRight(b.String(), "\n")
}

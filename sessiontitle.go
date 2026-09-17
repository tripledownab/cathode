// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---- naming a session (`/title`) ----
//
// The resume picker labelled every row with the first thing you asked. That is
// a poor label for what a long session turned into, and on a narrow terminal it
// is the only part of the row worth reading, so it is what gets truncated last
// and noticed first. A title you choose replaces it.
//
// Titles live in cathode's own session store, not the agent's transcript:
// neither backend has a place to put one, and a name is the user's note to
// themselves rather than anything the agent should see.

// titleMaxRunes caps what is stored. The picker truncates to the terminal
// anyway; this stops an accidental paste of a whole paragraph becoming the
// row's label, and it counts runes because a title is prose.
const titleMaxRunes = 72

// commitTitle names the live session, or clears the name when given no
// argument. Every path says what happened: a silent return reads as "saved".
func (m *model) commitTitle(arg string) tea.Cmd {
	title := trimTitle(arg)

	if m.session == "" {
		// A fresh session has no id until the agent reports one, and claude
		// does not until the first turn. A resumed session is seeded from the
		// id it was resumed with (model.go), so this is only the new-session
		// case.
		m.add(entError, "no session to name yet — send a turn first")
		return nil
	}
	// Make sure there is a row to title. A session started outside cathode is
	// listed from claude's own JSONL and may never have been written to our
	// store, and SetTitle ignores an id it does not know — so without this the
	// title is dropped while the confirmation below still prints.
	cwd, _ := os.Getwd()
	m.sessions.Touch(m.session, m.modelID, cwd, "", m.backend, time.Now())
	m.sessions.SetTitle(m.session, title)
	if title == "" {
		m.add(entInfo, "→ title cleared · the picker shows the first prompt again")
		return nil
	}
	m.add(entInfo, "→ title: "+title)
	return nil
}

// trimTitle normalises what the user typed: one line, no surrounding space,
// capped. Newlines collapse to spaces because the picker row is one line and a
// stored newline would silently truncate the rest of the title.
//
// The cap goes through trunc rather than slicing here, so it counts runes and
// marks what it dropped. A title cut mid-word with no ellipsis reads as one the
// user typed that way.
func trimTitle(s string) string {
	return trunc(strings.Join(strings.Fields(s), " "), titleMaxRunes)
}

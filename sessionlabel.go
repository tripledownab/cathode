// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
	"time"
)

// ---- how a session record reads on screen ----

// sessionLabelMax bounds what a session record persists as its label, and is
// deliberately well past any width the picker can render (pickerMaxWidth).
//
// It is a *storage* bound, not a display one: it stops a pasted essay becoming
// a row's label and bloating the store, and nothing more. Truncating for the
// screen is the picker's job, because only it knows the terminal's width — when
// this number did that job instead, a 64-character cut left a third of the row
// empty on a wide terminal.
const sessionLabelMax = 200

// truncFirst normalises a first prompt into a one-line session label. It is the
// row's title when no /title was set, so it is the part the list is read for.
func truncFirst(s string) string {
	// trunc, not a byte slice. s[:61] cuts a multi-byte rune in half and emits
	// invalid UTF-8, and a first prompt is prose — the same bug sysPromptSummary
	// was already fixed for.
	return trunc(strings.TrimSpace(strings.ReplaceAll(s, "\n", " ")), sessionLabelMax)
}

// humanizeAge renders "5m ago" / "2h ago" / "3d ago" style relative times.
func humanizeAge(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// short truncates an identifier to 8 chars (with "—" for empty), used for
// session IDs in picker rows, status bars, and sidebar headers.
func short(s string) string {
	if s == "" {
		return "—"
	}
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

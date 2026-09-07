// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

const testPrompt = "Be terse. Answer the question that was asked."

// The reminder exists only to back up a style claude actually loaded. An empty
// prompt means sysPromptArgs selected nothing, so there is nothing to remind
// about and the turn must go out untouched.
func TestWithReminderAppendsOnlyWhenAStyleIsLoaded(t *testing.T) {
	const turn = "explain the engine"

	if got := withReminder(turn, ""); got != turn {
		t.Errorf("no prompt: got %q, want the turn unchanged", got)
	}
	if got := withReminder(turn, "   \n\t "); got != turn {
		t.Errorf("blank prompt: got %q, want the turn unchanged", got)
	}

	got := withReminder(turn, testPrompt)
	if got == turn {
		t.Fatal("a loaded prompt appended nothing")
	}
	if stripReminder(got) != turn {
		t.Errorf("the user's text did not survive: got %q", stripReminder(got))
	}
	if !strings.Contains(got, defaultReminder) {
		t.Errorf("default reminder missing from %q", got)
	}
}

// A forwarded slash command is an argument list. Appending prose to it rewrites
// the arguments, so "/compact" would compact with our reminder as its
// instruction (see remind.go).
func TestWithReminderLeavesSlashCommandsAlone(t *testing.T) {
	for _, turn := range []string{"/compact", "/mcp", "/code-review high", "  /clear"} {
		if got := withReminder(turn, testPrompt); got != turn {
			t.Errorf("%q: got %q, want it unchanged", turn, got)
		}
	}
}

// A prompt that is not about length or format needs its own wording, so a
// marked block wins. Both malformed cases fall back rather than repeating the
// whole prompt on every turn.
func TestReminderTextPrefersAMarkedBlock(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		want   string
	}{
		{"no marker", testPrompt, defaultReminder},
		{"marked block", "Be terse.\n" + reminderOpen + "\nStay under four sentences.\n" + reminderClose + "\nMore prose.", "Stay under four sentences."},
		{"unterminated", "Be terse.\n" + reminderOpen + "\nStay under four sentences.", defaultReminder},
		{"empty block", "Be terse.\n" + reminderOpen + "\n \n" + reminderClose, defaultReminder},
	}
	for _, c := range cases {
		if got := reminderText(c.prompt); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// The tag is structural, not a keyword. Text that quotes it mid-prompt is the
// user's own writing and must survive — the same rule replay.go applies to
// claude's bookkeeping tags.
func TestStripReminderKeepsTextThatQuotesTheTag(t *testing.T) {
	quoted := "why does <" + reminderTag + ">hi</" + reminderTag + "> show up in my transcript?"
	if got := stripReminder(quoted); got != quoted {
		t.Errorf("got %q, want it unchanged", got)
	}
	plain := "no tags here at all"
	if got := stripReminder(plain); got != plain {
		t.Errorf("got %q, want it unchanged", got)
	}
	unclosed := "text with an <" + reminderTag + "> that never closes"
	if got := stripReminder(unclosed); got != unclosed {
		t.Errorf("got %q, want it unchanged", got)
	}
}

// Replay reads back the record claude stored, which is the text cathode sent.
// A resumed transcript has to show the prompt, not the reminder.
func TestReplayHidesTheReminder(t *testing.T) {
	const turn = "explain the engine"
	e, kind := replayUserText(withReminder(turn, testPrompt))
	if kind != replayShow {
		t.Fatalf("kind = %v, want replayShow", kind)
	}
	if e.kind != entUser || e.text != turn {
		t.Errorf("entry = %+v, want an entUser of %q", e, turn)
	}
}

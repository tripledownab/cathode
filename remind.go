// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import "strings"

// ---- the per-turn reminder ----
//
// outputstyle.go delivers the user's standing instructions, and the delivery is
// not in doubt: the text is visible in claude's system prompt, and the init line
// names the style claude resolved. Adherence is the part that fails. claude's
// own harness appends further response-shaping sections *after* the output
// style, so by the time a reply is composed the style no longer holds the last
// word, and the built-in guidance wins on length and format.
//
// This file gives the style the last word again. Every user turn carries a
// short reminder appended after the prompt, where recency favours it. That is
// the one lever cathode owns at runtime — the system prompt is fixed at launch
// (see sysprompt.go), but each turn is ours to compose.
//
// Three constraints shape it:
//
//   - It is not the user's words. So it is wrapped in a tag, which marks it as
//     out-of-band to the model and lets replay.go take it back out of the
//     transcript on the next resume.
//   - It rides the user turn. So it must never touch a slash command being
//     forwarded to claude: appending prose to "/compact" rewrites the command's
//     arguments.
//   - It repeats on every turn of a long session. So the default is two lines,
//     and a prompt that overrides it should stay about that size.

// reminderTag wraps the appended text. Both halves of the round trip
// (withReminder, stripReminder) derive from this one name.
const reminderTag = "cathode-reminder"

// Markers for an optional override block inside the prompt file. HTML comments
// because the prompt is markdown, and a comment renders as nothing in the
// output style claude reads.
const (
	reminderOpen  = "<!-- reminder -->"
	reminderClose = "<!-- /reminder -->"
)

// defaultReminder is what a prompt gets when it marks no block of its own. It
// does not restate the user's rules — cathode does not know which of them
// matter. It settles the precedence question instead, which is the actual
// failure: the style and the harness's later sections both describe how to
// reply, and the model needs to know which one governs.
const defaultReminder = "Your cathode output style governs this reply.\n" +
	"Where later instructions conflict with it on length, format or tone, the output style wins."

// reminderText returns the text to append for a given prompt, never "". A
// marked block in
// the prompt wins, so a user whose standing instructions are not about length
// or format can write their own one-liner. An unterminated marker falls back to
// the default: taking "the rest of the file" would repeat the whole prompt on
// every turn.
func reminderText(prompt string) string {
	i := strings.Index(prompt, reminderOpen)
	if i < 0 {
		return defaultReminder
	}
	rest := prompt[i+len(reminderOpen):]
	j := strings.Index(rest, reminderClose)
	if j < 0 {
		return defaultReminder
	}
	if body := strings.TrimSpace(rest[:j]); body != "" {
		return body
	}
	return defaultReminder
}

// withReminder returns the text to send for one user turn.
//
// prompt is model.sysPromptSeen: the standing instructions as the live claude
// got them at launch. It is empty unless the toggle is on *and* the file had
// text, which is exactly when an output style was selected — so there is no
// second setting to read here, and no way to remind about a style that claude
// never loaded.
func withReminder(text, prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return text
	}
	// A forwarded slash command is an argument list, not prose. Leave it alone.
	if strings.HasPrefix(strings.TrimSpace(text), "/") {
		return text
	}
	body := reminderText(prompt)
	return text + "\n\n<" + reminderTag + ">\n" + body + "\n</" + reminderTag + ">"
}

// stripReminder removes a trailing reminder block. Replay reads back the user
// record claude stored, which is the text cathode sent, reminder included. The
// resumed transcript has to show what the user typed.
//
// Only a block at the very end is ours. A tag with the user's own prose after
// it is the user quoting the tag, and it survives intact — the same rule
// replay.go:tagWrapped applies to claude's bookkeeping tags.
func stripReminder(text string) string {
	open, closer := "<"+reminderTag+">", "</"+reminderTag+">"
	i := strings.LastIndex(text, open)
	if i < 0 {
		return text
	}
	rest := text[i+len(open):]
	j := strings.Index(rest, closer)
	if j < 0 {
		return text
	}
	if strings.TrimSpace(rest[j+len(closer):]) != "" {
		return text
	}
	return strings.TrimRight(text[:i], " \t\r\n")
}

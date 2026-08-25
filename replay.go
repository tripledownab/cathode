// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// claude's session JSONL keeps user records the interactive UI never prints as
// prose: the caveat that precedes a local command, the slash command itself,
// its stdout, background-task notifications, skill preambles. The live stream
// never shows them either — stream.go surfaces only tool_result blocks out of a
// user event — so the replayed transcript has to hide them too. Left alone they
// come back as a screenful of raw <command-name> and <summary> markup on the
// next resume, which is the bug this file exists to prevent.

// replayKind says how one text block of a user record should surface.
type replayKind int

const (
	replaySkip   replayKind = iota // CLI bookkeeping: show nothing
	replayShow                     // a real prompt, or the slash command line
	replayStdout                   // a local command's own output
)

// replayUserText projects one text block of a user record into a transcript
// entry, and reports which kind of record it came from.
func replayUserText(text string) (entry, replayKind) {
	t := strings.TrimSpace(text)
	if t == "" {
		return entry{}, replaySkip
	}
	if !tagWrapped(t) {
		return entry{kind: entUser, text: t}, replayShow
	}
	// A slash command the user ran. Show the command line, which is what the
	// live run shows — cathode logs the typed text before forwarding it.
	if name := tagValue(t, "command-name"); name != "" {
		line := name
		if args := tagValue(t, "command-args"); args != "" {
			line += " " + args
		}
		return entry{kind: entUser, text: line}, replayShow
	}
	// The command's own output: "Set model to …", "Login successful". It can
	// carry SGR sequences, which would leak into the transcript's own styling.
	if out := tagValue(t, "local-command-stdout"); out != "" {
		return entry{kind: entInfo, text: ansi.Strip(out)}, replayStdout
	}
	return entry{}, replaySkip
}

// tagWrapped reports whether text is nothing but <tag>…</tag> wrappers and the
// whitespace between them. The test is structural on purpose. A prompt that
// quotes one of these tags inside a sentence is the user's own words, and it
// stays untouched.
func tagWrapped(t string) bool {
	for t != "" {
		if !strings.HasPrefix(t, "<") {
			return false
		}
		end := strings.IndexByte(t, '>')
		if end < 0 {
			return false
		}
		name := t[1:end]
		if name == "" || strings.ContainsAny(name, " \t\n\r/<") {
			return false
		}
		closer := strings.Index(t, "</"+name+">")
		if closer < 0 {
			return false
		}
		t = strings.TrimSpace(t[closer+len(name)+3:])
	}
	return true
}

// tagValue returns the text inside the first <name>…</name> pair. It returns
// "" when the tag is absent or empty — both mean "nothing to show" here.
func tagValue(t, name string) string {
	open, closer := "<"+name+">", "</"+name+">"
	i := strings.Index(t, open)
	if i < 0 {
		return ""
	}
	rest := t[i+len(open):]
	j := strings.Index(rest, closer)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:j])
}

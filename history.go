// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"sync"
)

// maxHistoryEntries caps the JSONL file so it can't grow unbounded over months
// of use. Matches opencode's MAX_HISTORY_ENTRIES.
const maxHistoryEntries = 50

// promptEntry is one persisted prompt. We deliberately keep it minimal — no
// timestamps, no parts — so the file stays grep-friendly and replaying old
// entries doesn't depend on schema migrations.
type promptEntry struct {
	Input string `json:"input"`
}

// history is the Ctrl-Up/Down recall buffer. Persisted as JSONL at
// $XDG_STATE_HOME/cathode/prompt-history.jsonl (or ~/.local/state/cathode/
// when XDG_STATE_HOME is unset).
//
// entries is a cache of that file, not the store: every running cathode
// instance shares it, so the write rules in historyfile.go are what keep one
// window from erasing another's prompts. This comment used to claim appends
// "touch only the tail in the common case", which is what hid the fact that
// past the cap they did not — see trim.
type history struct {
	mu      sync.Mutex
	entries []promptEntry
	cursor  int // 0 = live input; -1 = newest entry; -N = Nth from the end
	path    string
}

// openHistory loads from disk. A failure to resolve the path returns a working
// history with persistence disabled rather than aborting startup — losing the
// recall feature is worth far less than losing the whole TUI.
func openHistory() *history {
	path, err := historyPath()
	if err != nil {
		return &history{}
	}
	h := &history{path: path}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.load() // held-lock, like every other caller
	return h
}

// historyPath resolves $XDG_STATE_HOME/cathode/prompt-history.jsonl, creating
// the directory if needed.
func historyPath() (string, error) {
	return stateFilePath("prompt-history.jsonl")
}

// Append records a new prompt. Adjacent duplicates are dropped (re-sending the
// same prompt three times leaves one entry). Resets the walk cursor to live.
//
// "Adjacent" is now adjacent in the shared file, not in this window's own
// entries, because load reads what every window appended. So sending a prompt
// another window just sent records nothing new — the history already ends with
// that line, which is what the rule was always for.
func (h *history) Append(input string) {
	if strings.TrimSpace(input) == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if last := len(h.entries) - 1; last >= 0 && h.entries[last].Input == input {
		h.cursor = 0
		return
	}
	h.entries = append(h.entries, promptEntry{Input: input})
	h.cursor = 0
	if h.path == "" {
		return
	}
	h.appendLine(input)
	h.load() // re-read: the file also holds what the other windows appended
}

// Rewind puts the walk cursor back at live. Call it when the prompt is cleared
// without being sent (Ctrl-C): Move refuses to walk on from a recalled entry the
// user has since edited, and an emptied prompt is exactly that. Nil-safe.
func (h *history) Rewind() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cursor = 0
}

// Move walks the buffer. direction is -1 for "back into the past" (Ctrl-Up)
// or +1 for "forward toward live" (Ctrl-Down). currentInput protects in-flight
// typing: at the live cursor with text typed → no-op; on a recalled entry that
// has been edited → no-op. The (string, bool) result is (newInput, applied);
// when applied is false, the UI leaves the input unchanged.
func (h *history) Move(direction int, currentInput string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.entries) == 0 {
		return "", false
	}
	cur := h.cursor
	if cur < 0 {
		recalled := h.entries[len(h.entries)+cur].Input
		if recalled != currentInput {
			return "", false
		}
	} else if currentInput != "" {
		return "", false
	}
	next := cur + direction
	if next > 0 || -next > len(h.entries) {
		return "", false
	}
	h.cursor = next
	if next == 0 {
		return "", true
	}
	return h.entries[len(h.entries)+next].Input, true
}

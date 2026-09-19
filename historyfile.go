// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
)

// ---- how prompt history reaches the disk ----
//
// The file is shared with every other cathode instance, so a prompt is appended
// to it and never written as part of a rebuild of it. O_APPEND is what lets all
// of them write it with no lock and no lost line.
//
// The one write that does replace the whole file is the cap self-heal in load,
// and it re-reads under the lock first, for the reason sessionStore documents.

// load brings the cache up to date with the file, and holds the file to the cap.
// Held-lock: callers must hold h.mu.
//
// It runs after every append, not only at startup, because the file is shared: a
// prompt typed in another window belongs to this window's recall too, exactly as
// it does after a restart. The cap is small enough that an established history
// reaches this self-heal on nearly every turn, which is why it must start from
// what is on disk — writing this instance's view instead erased every prompt the
// other windows had appended since it started.
func (h *history) load() {
	if h.path == "" {
		return
	}
	if unlock, err := lockState(h.path + ".lock"); err == nil {
		defer unlock()
	}
	lines := readHistory(h.path)
	if len(lines) > maxHistoryEntries {
		lines = lines[len(lines)-maxHistoryEntries:]
		writeJSONL(h.path, lines)
	}
	h.entries = lines
}

// readHistory reads the prompts in the order they were sent. A record with no
// text is dropped — there is nothing to recall.
func readHistory(path string) []promptEntry {
	var out []promptEntry
	for _, e := range readJSONL[promptEntry](path) {
		if e.Input != "" {
			out = append(out, e)
		}
	}
	return out
}

// appendLine writes one prompt to the end of the file. Held-lock.
func (h *history) appendLine(input string) {
	f, err := os.OpenFile(h.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	b, err := json.Marshal(promptEntry{Input: input})
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
}

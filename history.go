package main

import (
	"bufio"
	"encoding/json"
	"os"
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
// when XDG_STATE_HOME is unset). Reads happen once at startup; appends touch
// only the tail in the common case, so it's safe to share across runs of the
// same session.
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
	h.load()
	return h
}

// historyPath resolves $XDG_STATE_HOME/cathode/prompt-history.jsonl, creating
// the directory if needed.
func historyPath() (string, error) {
	return stateFilePath("prompt-history.jsonl")
}

// load reads the JSONL and silently drops malformed lines (a previous crash
// mid-write shouldn't break recall). If the file is over the cap, we rewrite it
// trimmed — opencode does the same self-heal.
func (h *history) load() {
	f, err := os.Open(h.path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lines []promptEntry
	for sc.Scan() {
		var e promptEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err == nil && e.Input != "" {
			lines = append(lines, e)
		}
	}
	if len(lines) > maxHistoryEntries {
		lines = lines[len(lines)-maxHistoryEntries:]
		h.entries = lines
		h.rewrite()
		return
	}
	h.entries = lines
}

// rewrite atomically replaces the file with the in-memory entries. Only used
// for the cap-trim self-heal, never on the hot path.
func (h *history) rewrite() {
	if h.path == "" {
		return
	}
	tmp := h.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	for _, e := range h.entries {
		b, _ := json.Marshal(e)
		_, _ = f.Write(append(b, '\n'))
	}
	_ = f.Close()
	_ = os.Rename(tmp, h.path)
}

// Append records a new prompt. Adjacent duplicates are dropped (re-sending the
// same prompt three times leaves one entry). Resets the walk cursor to live.
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
	if len(h.entries) > maxHistoryEntries {
		h.entries = h.entries[len(h.entries)-maxHistoryEntries:]
		h.rewrite()
		return
	}
	if h.path == "" {
		return
	}
	f, err := os.OpenFile(h.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(promptEntry{Input: input})
	_, _ = f.Write(append(b, '\n'))
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

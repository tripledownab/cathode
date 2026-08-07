package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// listClaudeSessions enumerates claude's per-project session files for cwd,
// most-recently-modified first. The session ID is the filename stem; LastUsed
// is the file mtime (claude rewrites the JSONL on every turn, so mtime tracks
// last-activity accurately). Returns nil when the directory is missing — i.e.
// claude has never been run from this cwd. Filtering by cwd is implicit since
// claude already partitions by directory.
func listClaudeSessions(cwd string) []sessionInfo {
	dir, err := claudeProjectDir(cwd)
	if err != nil {
		return nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]sessionInfo, 0, len(ents))
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		first, model := parseSessionHead(filepath.Join(dir, name))
		out = append(out, sessionInfo{
			ID:       strings.TrimSuffix(name, ".jsonl"),
			Cwd:      cwd,
			LastUsed: fi.ModTime(),
			First:    truncFirst(first),
			Model:    model,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsed.After(out[j].LastUsed) })
	return out
}

// parseSessionHead scans the first records of a claude JSONL session and
// pulls out the user's opening prompt and the assistant model. Exits as soon
// as both are known so we never read the whole transcript (which can be
// hundreds of megabytes for long sessions).
func parseSessionHead(path string) (firstPrompt, model string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxRecordBytes)
	for sc.Scan() {
		var rec struct {
			Type    string `json:"type"`
			Message *struct {
				Role    string          `json:"role"`
				Model   string          `json:"model"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil || rec.Message == nil {
			continue
		}
		if model == "" && rec.Type == "assistant" && rec.Message.Model != "" {
			model = rec.Message.Model
		}
		if firstPrompt == "" && rec.Type == "user" && rec.Message.Role == "user" {
			firstPrompt = firstText(rec.Message.Content)
		}
		if firstPrompt != "" && model != "" {
			return
		}
	}
	return
}

// firstText pulls the first non-empty user-typed text out of a message's
// content, skipping non-text blocks (tool_result wrappers also show up under
// type="user" but aren't what the human typed). Both of claude's content
// encodings are handled by contentBlocks (transcript.go) — this and the
// transcript replay must decode identically or the picker and the replay
// disagree about which sessions have any conversation in them.
func firstText(raw json.RawMessage) string {
	for _, c := range contentBlocks(raw) {
		if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
			return c.Text
		}
	}
	return ""
}

// sessionItems projects sessions into picker rows. Claude's per-project JSONL
// directory is the primary source — that way sessions created by running
// `claude` directly (without going through doorway) show up too. Doorway's
// own store enriches the rows with cached metadata when present, and supplies
// store-only entries (matching the cwd filter) as a fallback. Empty cwd
// disables the store filter and skips the filesystem source, which the tests
// use to assert pure store behaviour.
func sessionItems(s *sessionStore, cwd string) []pickerItem {
	merged := mergeWithStore(listClaudeSessions(cwd), s, cwd)
	items := make([]pickerItem, 0, len(merged))
	for _, e := range merged {
		title := short(e.ID)
		if e.First != "" {
			title = short(e.ID) + "  " + e.First
		}
		parts := []string{filepath.Base(e.Cwd)}
		if e.Model != "" {
			parts = append(parts, e.Model)
		}
		parts = append(parts, humanizeAge(e.LastUsed))
		items = append(items, pickerItem{id: e.ID, title: title, subtitle: strings.Join(parts, " · ")})
	}
	return items
}

// mergeWithStore layers the doorway store on top of the filesystem list.
// Filesystem entries win on LastUsed (claude's mtime is always current) but
// take cached model/first-prompt from the store when absent. Store-only IDs
// for the same cwd are appended so re-execs and crashed sessions don't
// vanish from the picker just because the JSONL was rotated.
func mergeWithStore(fs []sessionInfo, s *sessionStore, cwd string) []sessionInfo {
	stored := make(map[string]sessionInfo)
	for _, e := range s.All() {
		stored[e.ID] = e
	}
	seen := make(map[string]bool, len(fs))
	out := make([]sessionInfo, 0, len(fs))
	for _, e := range fs {
		if cached, ok := stored[e.ID]; ok {
			if e.Model == "" && cached.Model != "" {
				e.Model = cached.Model
			}
			if e.First == "" && cached.First != "" {
				e.First = cached.First
			}
		}
		seen[e.ID] = true
		out = append(out, e)
	}
	want := filepath.Clean(cwd)
	for _, e := range s.All() {
		if seen[e.ID] {
			continue
		}
		if cwd != "" && filepath.Clean(e.Cwd) != want {
			continue
		}
		out = append(out, e)
	}
	return out
}

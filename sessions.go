package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// sessionInfo is one entry in the resume index. ID is what we feed to
// `claude --resume`; the rest is metadata to render in the picker.
type sessionInfo struct {
	ID       string    `json:"id"`
	Model    string    `json:"model"`
	Cwd      string    `json:"cwd"`
	LastUsed time.Time `json:"last_used"`
	First    string    `json:"first,omitempty"` // truncated first user prompt, if known
}

// sessionStore is the on-disk session index, persisted as JSONL at
// $XDG_STATE_HOME/cathode/sessions.jsonl. The file is rewritten atomically on
// every Touch so it stays one line per session — simpler than a log-compaction
// step and the file stays small (one entry per session we've ever seen).
type sessionStore struct {
	mu      sync.Mutex
	entries map[string]sessionInfo
	path    string
}

func openSessionStore() *sessionStore {
	path, err := sessionsPath()
	if err != nil {
		return &sessionStore{entries: map[string]sessionInfo{}}
	}
	s := &sessionStore{entries: map[string]sessionInfo{}, path: path}
	s.load()
	return s
}

func sessionsPath() (string, error) {
	return stateFilePath("sessions.jsonl")
}

func (s *sessionStore) load() {
	f, err := os.Open(s.path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e sessionInfo
		if err := json.Unmarshal(sc.Bytes(), &e); err == nil && e.ID != "" {
			s.entries[e.ID] = e
		}
	}
}

// Touch upserts a session. Empty model/cwd/first don't overwrite existing
// values (so a follow-up Touch carrying only LastUsed preserves prior
// metadata). LastUsed is always bumped.
func (s *sessionStore) Touch(id, model, cwd, first string, now time.Time) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.entries[id]
	cur.ID = id
	if model != "" {
		cur.Model = model
	}
	if cwd != "" {
		cur.Cwd = cwd
	}
	if first != "" && cur.First == "" {
		cur.First = first
	}
	cur.LastUsed = now
	s.entries[id] = cur
	s.rewrite()
}

// All returns sessions sorted most-recent first.
func (s *sessionStore) All() []sessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]sessionInfo, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsed.After(out[j].LastUsed) })
	return out
}

// rewrite is held-lock; callers must hold s.mu. Atomic via tmp + rename so a
// crash mid-write can't leave the file half-truncated.
func (s *sessionStore) rewrite() {
	if s.path == "" {
		return
	}
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	for _, e := range s.entries {
		b, _ := json.Marshal(e)
		_, _ = f.Write(append(b, '\n'))
	}
	_ = f.Close()
	_ = os.Rename(tmp, s.path)
}

// ---- presentation helpers (display formatting) ----

// truncFirst trims a first-prompt so the picker subtitle stays one line.
func truncFirst(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > 64 {
		return s[:61] + "…"
	}
	return s
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

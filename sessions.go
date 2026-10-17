// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"sort"
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
	// Title is a name the user gave this session (/title). It replaces First in
	// the picker, because a first prompt is often a poor label for what the
	// session turned into — and on a narrow terminal it is the row's only
	// readable part.
	Title string `json:"title,omitempty"`
	// Backend is which agent CLI owns this id. Empty means claude: every record
	// written before cathode had a second backend is one of its sessions, so the
	// zero value is the right default and no migration is needed.
	Backend string `json:"backend,omitempty"`
}

// sessionBackend normalises a stored value. Kept as one function because the
// empty-means-claude rule is read in two places and must not drift.
func sessionBackend(v string) string {
	if v == "" {
		return backendClaude
	}
	return v
}

// sessionStore is the session index, persisted as JSONL at
// $XDG_STATE_HOME/cathode/sessions.jsonl, one line per session ever seen.
//
// The *file* is the state and entries is only a cache of it. That distinction is
// the whole design, and getting it wrong lost user data: several cathode
// instances run at once, and a write that rebuilt the file from a map loaded at
// process start erased every row another instance had written since. A row is
// only ever written by the instance whose session it is, so the loser of that
// race lost the row outright — titles vanished, and store-only sessions
// disappeared from the picker. It was invisible for as long as the store was
// tested one process at a time.
//
// So every write re-reads the file under an exclusive lock, and every read
// re-reads it too. The file holds one line per session, and a turn is nowhere
// near hot enough for that to cost anything.
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	return s
}

func sessionsPath() (string, error) {
	return stateFilePath("sessions.jsonl")
}

// SetTitle names a session. An empty title clears it, so the picker falls back
// to the first prompt — there is no separate "unset" verb to remember.
func (s *sessionStore) SetTitle(id, title string) {
	if id == "" {
		return
	}
	s.mutate(func(m map[string]sessionInfo) {
		cur, ok := m[id]
		if !ok {
			// Titling a session the store has never seen would create a row with
			// no cwd, model or timestamp, which then sorts and renders as a ghost.
			return
		}
		cur.Title = title
		m[id] = cur
	})
}

// Touch upserts a session. Empty model/cwd/first don't overwrite existing
// values (so a follow-up Touch carrying only LastUsed preserves prior
// metadata). LastUsed is always bumped.
func (s *sessionStore) Touch(id, model, cwd, first, backend string, now time.Time) {
	if id == "" {
		return
	}
	s.mutate(func(m map[string]sessionInfo) {
		cur := m[id]
		cur.ID = id
		if backend != "" {
			cur.Backend = backend
		}
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
		m[id] = cur
	})
}

// All returns sessions sorted most-recent first. It re-reads the file, because
// the picker has to list what the other instances recorded too — a session
// started in another window is exactly the one you came to the picker for.
func (s *sessionStore) All() []sessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	out := make([]sessionInfo, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsed.After(out[j].LastUsed) })
	return out
}

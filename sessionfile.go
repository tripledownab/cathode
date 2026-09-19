// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

// ---- how the session store reaches the disk ----
//
// The rules that make a shared file safe live here, together, because missing
// one of them is what lost the user's session titles (sessionStore documents
// what happened). Nothing outside this file may write the store.

// refresh reloads the cache from the file. Held-lock: callers must hold s.mu.
// With no path there is no file to read, and the cache is the whole store.
func (s *sessionStore) refresh() {
	if s.path == "" {
		return
	}
	s.entries = readSessions(s.path)
}

// readSessions reads the store, keyed by id. A record with no id is dropped: it
// cannot be resumed or matched to anything, and it would render as a ghost row.
// A later line for the same id wins, so a file that was appended to rather than
// replaced still reads as one record per session.
func readSessions(path string) map[string]sessionInfo {
	out := map[string]sessionInfo{}
	for _, e := range readJSONL[sessionInfo](path) {
		if e.ID != "" {
			out[e.ID] = e
		}
	}
	return out
}

// mutate applies fn to the store and writes the result. Every write goes
// through here; nothing else may write the file.
//
// The re-read inside the lock is the point: it carries through the rows another
// instance has written since this one loaded, instead of replacing them with a
// stale snapshot.
func (s *sessionStore) mutate(fn func(map[string]sessionInfo)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.path == "" {
		fn(s.entries) // no file to share, so nothing to lock or re-read
		return
	}
	// A lock we could not take is not a reason to drop what the user did. The
	// read-modify-write still runs, which leaves the far narrower race that
	// Windows has all the time (statelock_windows.go).
	if unlock, err := lockState(s.path + ".lock"); err == nil {
		defer unlock()
	}
	s.refresh()
	fn(s.entries)
	s.write()
}

// write replaces the file with the cache. Held-lock, and called only from
// mutate, which has just re-read what this is about to replace.
func (s *sessionStore) write() {
	rows := make([]sessionInfo, 0, len(s.entries))
	for _, e := range s.entries {
		rows = append(rows, e)
	}
	writeJSONL(s.path, rows)
}

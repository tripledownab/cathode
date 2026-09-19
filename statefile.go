// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

// ---- reading and replacing a JSONL state file ----
//
// Both persisted stores are JSONL under $XDG_STATE_HOME/cathode, and both are
// shared by every running cathode instance. The two operations that has to get
// right live here rather than once per store, because a copy differing by one
// detail is how a shared file loses data quietly: the session store and the
// prompt history each grew their own, and only one of them ended up with a
// unique temp name.

// maxStateLine bounds one record. A pasted prompt is the longest thing either
// store holds, and a longer line is dropped rather than growing the scanner
// without limit.
const maxStateLine = 1024 * 1024

// readJSONL parses one record per line. A missing file is an empty store, and a
// line that will not parse is skipped rather than failing the whole read — one
// record left half-written by a crash must not cost the user the rest.
func readJSONL[T any](path string) []T {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxStateLine)
	var out []T
	for sc.Scan() {
		var rec T
		if json.Unmarshal(sc.Bytes(), &rec) == nil {
			out = append(out, rec)
		}
	}
	return out
}

// writeJSONL replaces path with one JSON line per record.
//
// Atomic via a temp file and a rename, so a crash cannot leave the file
// half-truncated and a concurrent reader sees either the old file or the new
// one. The temp name is unique: two instances sharing one "<name>.tmp" write
// into the same file and rename a half-finished one into place, which is the one
// way a whole store goes at once.
//
// Callers must hold the file lock (lockState) and must have re-read the file
// inside it. Replacing a file that several instances write means starting from
// what is on disk, not from a cache loaded at process start — see sessionStore
// for what that cost.
func writeJSONL[T any](path string, records []T) {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return
	}
	for _, r := range records {
		b, err := json.Marshal(r)
		if err != nil {
			continue
		}
		if _, err := tmp.Write(append(b, '\n')); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return
		}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	if os.Rename(tmp.Name(), path) != nil {
		_ = os.Remove(tmp.Name())
	}
}

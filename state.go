package main

import (
	"errors"
	"os"
	"path/filepath"
)

// appStateName is the directory under the XDG state root that holds the resume
// index, prompt history, and settings. It moved from "doorway" to "cathode"
// with the rename; migrateLegacyState relocates an old dir on first run so
// existing sessions/history/settings survive. legacyStateName is the old name.
const (
	appStateName    = "cathode"
	legacyStateName = "doorway"
)

// errNoStateDir signals that no XDG state root was resolvable (no
// XDG_STATE_HOME and no home dir). Callers treat it as "persistence disabled"
// and keep running with in-memory state.
var errNoStateDir = errors.New("no resolvable state directory")

// stateBase resolves the XDG state root: $XDG_STATE_HOME, else ~/.local/state.
// Returns "" only when neither is resolvable.
func stateBase() string {
	if base := os.Getenv("XDG_STATE_HOME"); base != "" {
		return base
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state")
}

// stateDir returns the app state directory ($base/cathode), creating it — and
// migrating the legacy $base/doorway dir on the way — as a side effect. Returns
// "" if no base is resolvable or the directory can't be created, which the
// path helpers turn into errNoStateDir.
func stateDir() string {
	base := stateBase()
	if base == "" {
		return ""
	}
	migrateLegacyState(base)
	dir := filepath.Join(base, appStateName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	return dir
}

// migrateLegacyState renames $base/doorway → $base/cathode when the old dir
// exists and the new one doesn't. Idempotent and cheap (two stats in the steady
// state), so it's safe to call on every stateDir lookup. A pre-existing cathode
// dir wins — we never clobber it — and any rename error is swallowed so a failed
// migration just starts the app fresh rather than aborting startup.
func migrateLegacyState(base string) {
	newDir := filepath.Join(base, appStateName)
	if _, err := os.Stat(newDir); err == nil {
		return // already migrated (or fresh install) — nothing to do
	}
	oldDir := filepath.Join(base, legacyStateName)
	if _, err := os.Stat(oldDir); err != nil {
		return // no legacy dir to move
	}
	_ = os.Rename(oldDir, newDir)
}

// stateFilePath joins name onto the app state dir, or returns errNoStateDir when
// persistence is unavailable. Shared by the sessions/history/settings stores.
func stateFilePath(name string) (string, error) {
	dir := stateDir()
	if dir == "" {
		return "", errNoStateDir
	}
	return filepath.Join(dir, name), nil
}

package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// maxRecordBytes caps one line of a claude session JSONL. Tool results are the
// long pole (a whole file read lands in one record), so the default 64K scanner
// limit is nowhere near enough.
const maxRecordBytes = 16 << 20

// cwdProbeLines is how far into a session file we look for the cwd stamp before
// moving on. claude writes "cwd" on nearly every record, but the head of a file
// is queue-operation bookkeeping that carries none — in practice it shows up by
// line three.
const cwdProbeLines = 20

// projectSlug encodes a cwd the way claude names its per-project session
// directory: every character outside [A-Za-z0-9-] becomes "-", one dash per
// character — runs are never collapsed. So `/Users/w/Work/Triple Down/web`
// slugs to `-Users-w-Work-Triple-Down-web` (the space is a dash like the
// slashes), and `/Users/w/.config/wezterm` to `-Users-w--config-wezterm` (the
// "/" and the "." each contribute one).
//
// This used to replace only "/", which silently broke every project whose path
// contains a space, dot, or underscore: the lookup pointed at a directory that
// does not exist, so the ctrl+r picker lost claude's own sessions and a resumed
// session replayed an empty transcript.
func projectSlug(cwd string) string {
	var b strings.Builder
	b.Grow(len(cwd))
	for _, r := range cwd {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// claudeProjectsRoot is ~/.claude/projects, the parent of every per-project
// session directory.
func claudeProjectsRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// claudeProjectDir resolves the directory where claude persists one JSONL per
// session for cwd — the path both the resume picker (claude_sessions.go) and
// the transcript replay (transcript.go) read from.
//
// The slug is the fast path. When it misses we ask the sessions themselves,
// since every record is stamped with the cwd it was recorded in; that keeps
// resume working even where claude's naming rule and ours disagree on some
// character (non-ASCII, say) that hasn't been pinned down. If nothing matches,
// the slug path comes back anyway so callers simply fail to open it — a project
// claude has never seen degrades to "no sessions", not an error.
func claudeProjectDir(cwd string) (string, error) {
	root, err := claudeProjectsRoot()
	if err != nil {
		return "", err
	}
	slugged := filepath.Join(root, projectSlug(cwd))
	if fi, err := os.Stat(slugged); err == nil && fi.IsDir() {
		return slugged, nil
	}
	if dir := findProjectDirByCwd(root, cwd); dir != "" {
		return dir, nil
	}
	return slugged, nil
}

// findProjectDirByCwd scans ~/.claude/projects for the directory whose sessions
// were recorded in cwd. Only reached when the slug lookup missed, so the cost —
// a partial read of one file per project — stays off the common path.
func findProjectDirByCwd(root, cwd string) string {
	ents, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	want := filepath.Clean(cwd)
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if recordedCwd(dir) == want {
			return dir
		}
	}
	return ""
}

// recordedCwd returns the first cwd stamped into any session file in dir, or ""
// if the directory holds no readable session. Files are tried in turn so one
// truncated JSONL doesn't disqualify the whole project.
func recordedCwd(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if c := sessionCwd(filepath.Join(dir, e.Name())); c != "" {
			return c
		}
	}
	return ""
}

// sessionCwd reads the cwd stamp off the head of one session JSONL.
func sessionCwd(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxRecordBytes)
	for i := 0; i < cwdProbeLines && sc.Scan(); i++ {
		var rec struct {
			Cwd string `json:"cwd"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err == nil && rec.Cwd != "" {
			return filepath.Clean(rec.Cwd)
		}
	}
	return ""
}

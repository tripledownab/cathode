package main

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// maxRepoFiles bounds how many candidate paths the @-completion loads, so a
// giant working tree can't blow up memory or the fuzzy filter.
const maxRepoFiles = 20000

// loadRepoFiles is the @-completion's file source, a package var so tests can
// stub it with a fixed list instead of shelling out to git.
var loadRepoFiles = gitRepoFiles

// gitRepoFiles lists tracked plus untracked-but-not-ignored paths relative to
// the cwd via `git ls-files`, which respects .gitignore for free (and mirrors
// what Claude Code's own @-picker offers). Outside a git repo, or if git is
// missing, it falls back to a plain directory walk. Paths come NUL-separated so
// names with spaces or newlines survive; the list is sorted for a stable menu
// order on an empty query.
func gitRepoFiles() []string {
	out, err := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z").Output()
	if err != nil {
		return walkFiles(".")
	}
	files := make([]string, 0, 256)
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" {
			continue
		}
		files = append(files, p)
		if len(files) >= maxRepoFiles {
			break
		}
	}
	sort.Strings(files)
	return files
}

// walkFiles is the non-git fallback: a walk of root that skips VCS and
// dependency dirs so the menu isn't drowned in build output. Best-effort — it
// doesn't read .gitignore, which is why git is preferred.
func walkFiles(root string) []string {
	skip := map[string]bool{".git": true, "node_modules": true, "vendor": true, ".venv": true}
	var files []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, strings.TrimPrefix(p, "./"))
		if len(files) >= maxRepoFiles {
			return fs.SkipAll
		}
		return nil
	})
	sort.Strings(files)
	return files
}

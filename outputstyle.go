// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ---- the standing instructions, delivered as a claude *output style* ----
//
// The first version passed --append-system-prompt-file. The text arrived (it is
// visible in the model's system prompt) but the reply style often ignored it:
// appended text lands after claude's own response-style section, so the two
// compete and the built-in one usually wins. An output style replaces that
// section instead of arguing with it.
//
// Three facts about output styles, verified against CLI 2.1.228, decide the
// shape of this file:
//
//   - claude resolves a style by the `name` in its frontmatter, not by the
//     filename. A file named probefile.md whose frontmatter says another name
//     does not answer to "probefile".
//   - an unknown style name is ignored in silence — no warning, no error, just
//     the default behaviour back. That is the same symptom as a prompt that
//     does not apply, so nothing here may derive one name from another: the
//     filename, the frontmatter name and the --settings value are all
//     outputStyleName.
//   - --settings takes JSON on the command line, so the style needs no entry in
//     the user's settings.json and their interactive claude is untouched.
//
// The style file goes in claude's own config dir. A project-level
// .claude/output-styles/ would work too, but cathode runs in the user's repo,
// and writing a file into whatever repo you happen to open is not ours to do.

const outputStyleName = "cathode"

// claudeConfigDir is claude's config dir: $CLAUDE_CONFIG_DIR when set, else
// ~/.claude. Returns "" when the home dir is unresolvable.
func claudeConfigDir() string {
	if d := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// outputStylePath is the managed style file, or "" if the config dir is
// unresolvable.
func outputStylePath() string {
	d := claudeConfigDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "output-styles", outputStyleName+".md")
}

// outputStyleDoc renders the style file: the frontmatter claude parses, then
// the user's prose verbatim. The description says where the source text lives,
// because this file is generated and an edit here is lost on the next launch.
func outputStyleDoc(text string) string {
	return "---\n" +
		"name: " + outputStyleName + "\n" +
		"description: standing instructions, written by cathode — edit " + sysPromptFile + " in the state dir instead\n" +
		"---\n\n" +
		strings.TrimSpace(text) + "\n"
}

// writeOutputStyle renders the prompt text into the style file, creating the
// directory. main calls it once per launch, so an edited prompt file applies on
// the restart /sysprompt already performs.
func writeOutputStyle(text string) error {
	p := outputStylePath()
	if p == "" {
		return errors.New("cannot resolve claude's config dir")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(outputStyleDoc(text)), 0o644)
}

// initStyleNote reads the style claude reports in its system/init line and says
// what the session line should carry. want is the /sysprompt toggle.
//
// seg names the live style, for any style but the default — the user's own
// selection counts, not just ours. warn fires when the toggle is on and claude
// resolved some other style, which is the silent failure this whole field
// exists to catch: a style claude cannot find produces no warning of its own,
// just default behaviour that reads as "my instructions are ignored".
//
// An empty report means the CLI predates the field. Say nothing then — a false
// alarm about a working session is worse than no line at all.
func initStyleNote(reported string, want bool) (seg, warn string) {
	r := strings.TrimSpace(reported)
	if r == "" {
		return "", ""
	}
	if want && r != outputStyleName {
		warn = "extra system prompt is on, but claude loaded output style " + r +
			" — check " + outputStylePath()
	}
	if r != "default" {
		seg = " · style " + r
	}
	return seg, warn
}

// outputStyleSettings is the --settings value that selects the style. Marshaled
// rather than hand-written so the name can never be quoted wrong.
func outputStyleSettings() string {
	b, err := json.Marshal(map[string]string{"outputStyle": outputStyleName})
	if err != nil {
		return `{"outputStyle":"` + outputStyleName + `"}`
	}
	return string(b)
}

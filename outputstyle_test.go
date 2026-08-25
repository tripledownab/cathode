// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// frontmatterName reads the `name:` claude resolves the style by.
func frontmatterName(t *testing.T, doc string) string {
	t.Helper()
	if !strings.HasPrefix(doc, "---\n") {
		t.Fatalf("style file needs frontmatter, got %q", doc)
	}
	body, _, ok := strings.Cut(doc[4:], "\n---\n")
	if !ok {
		t.Fatalf("frontmatter is not closed: %q", doc)
	}
	for _, line := range strings.Split(body, "\n") {
		if v, ok := strings.CutPrefix(line, "name:"); ok {
			return strings.TrimSpace(v)
		}
	}
	t.Fatalf("frontmatter has no name: %q", body)
	return ""
}

// The one invariant that makes the feature work at all. claude resolves an
// output style by its frontmatter name, and selecting a name it cannot find is
// ignored in silence — same symptom as the prompt not applying. So the name in
// the file and the name in --settings must be the same string.
func TestOutputStyleNamesAgree(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())

	if got := frontmatterName(t, outputStyleDoc("Be terse.")); got != outputStyleName {
		t.Errorf("frontmatter name = %q, want %q", got, outputStyleName)
	}
	var sel map[string]string
	if err := json.Unmarshal([]byte(outputStyleSettings()), &sel); err != nil {
		t.Fatalf("--settings value is not JSON: %v", err)
	}
	if sel["outputStyle"] != outputStyleName {
		t.Errorf("--settings selects %q, want %q", sel["outputStyle"], outputStyleName)
	}
	if base := filepath.Base(outputStylePath()); base != outputStyleName+".md" {
		t.Errorf("style file is %q, want %q", base, outputStyleName+".md")
	}
}

// The prose is the user's. It goes in under the frontmatter, unedited.
func TestOutputStyleDocKeepsTheProse(t *testing.T) {
	text := "# Style\n\n- One instruction per sentence.\n- No semicolons.\n"
	doc := outputStyleDoc(text)

	_, body, ok := strings.Cut(doc, "\n---\n\n")
	if !ok {
		t.Fatalf("body should follow the frontmatter, got %q", doc)
	}
	if body != strings.TrimSpace(text)+"\n" {
		t.Errorf("body = %q, want the prompt text verbatim", body)
	}
	// The description points at the source file, because this one is generated.
	if !strings.Contains(doc, sysPromptFile) {
		t.Errorf("frontmatter should say where to edit the text, got %q", doc)
	}
}

func TestWriteOutputStyle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)

	// The output-styles dir won't exist on a machine that has never used one.
	if err := writeOutputStyle("Be terse."); err != nil {
		t.Fatalf("writeOutputStyle: %v", err)
	}
	p := filepath.Join(dir, "output-styles", outputStyleName+".md")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("style file not written: %v", err)
	}
	if !strings.Contains(string(b), "Be terse.") {
		t.Errorf("style file = %q, want the prompt text", string(b))
	}

	// An edited prompt replaces the old style. Appending would leave the
	// retracted rules in force.
	if err := writeOutputStyle("Answer in one line."); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	b, _ = os.ReadFile(p)
	if strings.Contains(string(b), "Be terse.") {
		t.Errorf("rewrite should replace the old text, got %q", string(b))
	}
}

// The init line is the only confirmation the CLI gives, so it has to read the
// same way in all four states.
func TestInitStyleNote(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())

	cases := []struct {
		name       string
		reported   string
		want       bool
		wantSeg    bool
		wantWarned bool
	}{
		{"applied", outputStyleName, true, true, false},
		{"toggle off, default style", "default", false, false, false},
		// The silent failure this field exists to catch.
		{"toggle on, style missing", "default", true, false, true},
		// A style the user selected themselves is worth naming, not warning about.
		{"toggle off, user's own style", "Explanatory", false, true, false},
		// An older CLI omits the field entirely — saying nothing beats guessing.
		{"field absent", "", true, false, false},
	}
	for _, c := range cases {
		seg, warn := initStyleNote(c.reported, c.want)
		if (seg != "") != c.wantSeg {
			t.Errorf("%s: seg = %q, want present=%v", c.name, seg, c.wantSeg)
		}
		if c.wantSeg && !strings.Contains(seg, c.reported) {
			t.Errorf("%s: seg %q should name the live style", c.name, seg)
		}
		if (warn != "") != c.wantWarned {
			t.Errorf("%s: warn = %q, want present=%v", c.name, warn, c.wantWarned)
		}
		if c.wantWarned && !strings.Contains(warn, outputStylePath()) {
			t.Errorf("%s: warn should say where to look, got %q", c.name, warn)
		}
	}
}

// End to end through the event router: the session line names the style, and a
// toggle that didn't land says so.
func TestInitEventReportsStyle(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())

	// Session "" keeps sessions.Touch a no-op, so a bare &model{} is safe.
	m := &model{settings: settings{SysPrompt: true}}
	m.handleEvent(Envelope{Type: "system", Subtype: "init", Model: "opus", OutputStyle: outputStyleName})
	if len(m.entries) != 1 {
		t.Fatalf("want just the session line, got %+v", m.entries)
	}
	if !strings.Contains(m.entries[0].text, "style "+outputStyleName) {
		t.Errorf("session line should name the style, got %q", m.entries[0].text)
	}

	missed := &model{settings: settings{SysPrompt: true}}
	missed.handleEvent(Envelope{Type: "system", Subtype: "init", Model: "opus", OutputStyle: "default"})
	if len(missed.entries) != 2 || missed.entries[1].kind != entError {
		t.Fatalf("a toggle that didn't land must be reported, got %+v", missed.entries)
	}
}

// Without CLAUDE_CONFIG_DIR the style has to land where claude actually looks.
func TestOutputStylePathDefaultsToHome(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	want := filepath.Join(home, ".claude", "output-styles", outputStyleName+".md")
	if got := outputStylePath(); got != want {
		t.Errorf("outputStylePath = %q, want %q", got, want)
	}
}

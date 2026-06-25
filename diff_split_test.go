package main

import (
	"regexp"
	"strings"
	"testing"
)

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// A changed line should render old and new beside each other on one row (the
// whole point of split), and the card should still tally the +/- counts.
func TestRenderDiffSplit(t *testing.T) {
	old := "func add(a, b int) int {\n\treturn a + b\n}\n"
	neu := "func add(a, b, c int) int {\n\treturn a + b + c\n}\n"
	out := stripANSI(renderDiffSplit("math.go", old, neu, 100))

	if !strings.Contains(out, "math.go") || !strings.Contains(out, "+2") || !strings.Contains(out, "-2") {
		t.Fatalf("missing title / counts:\n%s", out)
	}
	var paired bool
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "func add(a, b int)") && strings.Contains(ln, "func add(a, b, c int)") {
			paired = true
		}
	}
	if !paired {
		t.Fatalf("old and new signature not shown side-by-side:\n%s", out)
	}
}

// The /diff command opens the picker with no arg and rejects a bad one. (The
// valid-arg path calls commitDiff, which persists settings, so it's left to the
// settings wiring rather than exercised here.)
func TestDiffCommand(t *testing.T) {
	m := &model{w: 100, h: 30}
	nm, _, handled := runSlash(m, "/diff")
	if !handled || nm.picker == nil || nm.picker.kind != "diff" {
		t.Fatalf("/diff should open the diff picker (handled=%v picker=%v)", handled, nm.picker)
	}

	bad := &model{w: 100, h: 30}
	nb, _, _ := runSlash(bad, "/diff bogus")
	if nb.picker != nil {
		t.Error("/diff bogus should not open a picker")
	}
	if n := len(nb.entries); n == 0 || nb.entries[n-1].kind != entError {
		t.Error("/diff bogus should record an error entry")
	}
}

// renderDiffFor honors the style and falls back to unified when too narrow.
func TestRenderDiffForFallback(t *testing.T) {
	old, neu := "a\nb\n", "a\nc\n"
	if renderDiffFor(diffSplit, "f", old, neu, 60) != renderDiff("f", old, neu, 60) {
		t.Error("split below splitMinWidth should fall back to unified")
	}
	if renderDiffFor(diffSplit, "f", old, neu, 120) != renderDiffSplit("f", old, neu, 120) {
		t.Error("split at a wide width should render side-by-side")
	}
	if renderDiffFor(diffUnified, "f", old, neu, 120) != renderDiff("f", old, neu, 120) {
		t.Error("unified style should always render the single-column card")
	}
}

package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
)

func TestRebuildRendersMarkdown(t *testing.T) {
	m := newModel(&Engine{}, "ask", nil, "bar", "")
	m.vp = viewport.New(80, 24)
	m.ready = true
	m.makeRenderer()
	if m.md == nil {
		t.Fatal("glamour renderer not constructed")
	}
	m.add(entUser, "fix the bug in **main.go**")
	m.add(entClaude, "Here's a fix:\n\n```go\nfmt.Println(\"hi\")\n```\n\n- step one\n- step two")
	m.add(entTool, "Edit\n{\"file\":\"main.go\",\"old\":\"x\",\"new\":\"y\"}")
	m.add(entInfo, "— done · 0.0012 USD —")
	out := strings.ToLower(m.vp.View())
	if !strings.Contains(out, "claude") || !strings.Contains(out, "you") {
		t.Fatalf("expected role labels in output, got:\n%s", m.vp.View())
	}
	t.Logf("rendered %d entries OK", len(m.entries))
}

func TestDiffRendering(t *testing.T) {
	// Edit tool: before/after carried in the input
	in := []byte(`{"file_path":"main.go","old_string":"fmt.Println(\"a\")\nx := 1","new_string":"fmt.Println(\"b\")\nx := 2\ny := 3"}`)
	ds, ok := diffsForTool("Edit", in)
	if !ok || len(ds) != 1 {
		t.Fatalf("Edit not detected: ok=%v n=%d", ok, len(ds))
	}
	out := renderDiff(ds[0].file, ds[0].old, ds[0].new, 80)
	for _, want := range []string{"main.go", "+", "-", "│"} {
		if !strings.Contains(out, want) {
			t.Fatalf("diff missing %q in:\n%s", want, out)
		}
	}

	// MultiEdit: multiple hunks
	mi := []byte(`{"file_path":"a.txt","edits":[{"old_string":"foo","new_string":"bar"},{"old_string":"baz","new_string":"qux"}]}`)
	ds, ok = diffsForTool("MultiEdit", mi)
	if !ok || len(ds) != 2 {
		t.Fatalf("MultiEdit not detected: ok=%v n=%d", ok, len(ds))
	}

	// Non-edit tool falls through
	if _, ok := diffsForTool("Bash", []byte(`{"command":"ls"}`)); ok {
		t.Fatal("Bash should not produce a diff")
	}
	t.Log("diff rendering OK")
}

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	udiff "github.com/aymanbagabas/go-udiff"
)

// fileDiff is one before/after pair to visualise. A MultiEdit produces several.
type fileDiff struct{ file, old, new string }

// edit-tool input shapes from Claude Code.
type editInput struct {
	FilePath  string `json:"file_path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}
type writeInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}
type multiEditInput struct {
	FilePath string `json:"file_path"`
	Edits    []struct {
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	} `json:"edits"`
}

// diffsForTool returns the before/after pairs for an edit-style tool_use, or
// (nil,false) for anything that isn't a file edit (caller falls back to a plain
// tool card). For Write we read the current file off disk for the "before",
// which works because claude runs in our cwd.
func diffsForTool(name string, input json.RawMessage) ([]fileDiff, bool) {
	switch name {
	case "Edit":
		var in editInput
		if json.Unmarshal(input, &in) == nil && in.FilePath != "" {
			return []fileDiff{{in.FilePath, in.OldString, in.NewString}}, true
		}
	case "Write":
		var in writeInput
		if json.Unmarshal(input, &in) == nil && in.FilePath != "" {
			old, _ := os.ReadFile(in.FilePath) // empty if new file
			return []fileDiff{{in.FilePath, string(old), in.Content}}, true
		}
	case "MultiEdit":
		var in multiEditInput
		if json.Unmarshal(input, &in) == nil && in.FilePath != "" {
			var ds []fileDiff
			for _, e := range in.Edits {
				ds = append(ds, fileDiff{in.FilePath, e.OldString, e.NewString})
			}
			if len(ds) > 0 {
				return ds, true
			}
		}
	}
	return nil, false
}

// renderDiff builds one styled, line-numbered diff card.
func renderDiff(filename, oldText, newText string, width int) string {
	if width < 24 {
		width = 24
	}
	u := udiff.Unified("a/"+filename, "b/"+filename, oldText, newText)
	if strings.TrimSpace(u) == "" {
		return dBox.Width(width - 2).Render(dTitle.Render(" "+filename+" ") + "\n" + dCtx.Render("(no changes)"))
	}

	const gutterW = 11 // "%4s %4s│ "
	content := width - 4 - gutterW
	if content < 8 {
		content = 8
	}

	var body strings.Builder
	added, removed := 0, 0
	oldLn, newLn := 0, 0

	for _, raw := range strings.Split(u, "\n") {
		switch {
		case strings.HasPrefix(raw, "+++"), strings.HasPrefix(raw, "---"):
			continue
		case strings.HasPrefix(raw, "@@"):
			oldLn, newLn = parseHunk(raw)
			body.WriteString(dGutter.Render("         │ ") + dHunk.Render(trunc(raw, content)) + "\n")
		case strings.HasPrefix(raw, "+"):
			added++
			body.WriteString(gut("", newLn) + dAdd.Render(trunc("+ "+tabs(raw[1:]), content)) + "\n")
			newLn++
		case strings.HasPrefix(raw, "-"):
			removed++
			body.WriteString(gut(strconv.Itoa(oldLn), -1) + dDel.Render(trunc("- "+tabs(raw[1:]), content)) + "\n")
			oldLn++
		case strings.HasPrefix(raw, " "):
			body.WriteString(gut(strconv.Itoa(oldLn), newLn) + dCtx.Render(trunc("  "+tabs(raw[1:]), content)) + "\n")
			oldLn++
			newLn++
		}
	}

	title := fmt.Sprintf(" %s  %s %s ", filename,
		dAdd.Render("+"+strconv.Itoa(added)), dDel.Render("-"+strconv.Itoa(removed)))
	return dBox.Width(width - 2).Render(
		dTitle.Render(title) + "\n" + strings.TrimRight(body.String(), "\n"))
}

// gut renders the old/new line-number gutter. Pass an empty old or new==-1 to
// blank that column (for pure add/delete lines).
func gut(old string, newLn int) string {
	n := ""
	if newLn >= 0 {
		n = strconv.Itoa(newLn)
	}
	return dGutter.Render(fmt.Sprintf("%4s %4s│ ", old, n))
}

// parseHunk reads "@@ -a,b +c,d @@" and returns the old and new start lines.
func parseHunk(h string) (int, int) {
	var o, n int
	for _, p := range strings.Fields(h) {
		if strings.HasPrefix(p, "-") {
			o = atoiBefore(p[1:])
		} else if strings.HasPrefix(p, "+") {
			n = atoiBefore(p[1:])
		}
	}
	return o, n
}

func atoiBefore(s string) int {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	v, _ := strconv.Atoi(s)
	return v
}

func tabs(s string) string { return strings.ReplaceAll(s, "\t", "  ") }

func trunc(s string, w int) string {
	if w < 1 {
		w = 1
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

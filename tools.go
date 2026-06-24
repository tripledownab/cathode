package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// renderTool returns a typed card for a known tool name, or "" to let the
// caller fall back to the generic name + JSON card. Width is the column budget
// for the card (already net of the scrollbar gutter).
//
// Each branch decodes the documented input shape; if the JSON is malformed,
// the fallback path takes over so nothing crashes.
func renderTool(name string, input json.RawMessage, width int) string {
	switch name {
	case "Bash":
		var in struct {
			Command     string `json:"command"`
			Description string `json:"description"`
		}
		if json.Unmarshal(input, &in) != nil || in.Command == "" {
			return ""
		}
		return toolCard("⚙ Bash", in.Description, cYou.Render("$ ")+in.Command, width)

	case "Grep":
		var in struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
			Glob    string `json:"glob"`
			Type    string `json:"type"`
		}
		if json.Unmarshal(input, &in) != nil || in.Pattern == "" {
			return ""
		}
		where := in.Path
		if where == "" {
			where = "."
		}
		if in.Glob != "" {
			where += "  " + cDim.Render("("+in.Glob+")")
		} else if in.Type != "" {
			where += "  " + cDim.Render("(type "+in.Type+")")
		}
		body := cName.Render("/"+in.Pattern+"/") + cDim.Render("  in  ") + where
		return toolCard("⚙ Grep", "", body, width)

	case "Glob":
		var in struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if json.Unmarshal(input, &in) != nil || in.Pattern == "" {
			return ""
		}
		where := in.Path
		if where == "" {
			where = "."
		}
		body := cName.Render(in.Pattern) + cDim.Render("  under  ") + where
		return toolCard("⚙ Glob", "", body, width)

	case "Read":
		var in struct {
			FilePath string `json:"file_path"`
			Offset   int    `json:"offset"`
			Limit    int    `json:"limit"`
		}
		if json.Unmarshal(input, &in) != nil || in.FilePath == "" {
			return ""
		}
		hint := ""
		if in.Limit > 0 {
			hint = fmt.Sprintf("L%d-%d", in.Offset+1, in.Offset+in.Limit)
		} else if in.Offset > 0 {
			hint = fmt.Sprintf("from L%d", in.Offset+1)
		}
		body := in.FilePath
		if hint != "" {
			body += "  " + cDim.Render(hint)
		}
		return toolCard("⚙ Read", "", body, width)

	case "LS":
		var in struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(input, &in) != nil {
			return ""
		}
		p := in.Path
		if p == "" {
			p = "."
		}
		return toolCard("⚙ LS", "", p, width)

	case "WebFetch":
		var in struct {
			URL    string `json:"url"`
			Prompt string `json:"prompt"`
		}
		if json.Unmarshal(input, &in) != nil || in.URL == "" {
			return ""
		}
		body := cName.Render(in.URL)
		if in.Prompt != "" {
			body += "\n" + cDim.Render(trunc(in.Prompt, width-4))
		}
		return toolCard("⚙ WebFetch", "", body, width)

	case "WebSearch":
		var in struct {
			Query string `json:"query"`
		}
		if json.Unmarshal(input, &in) != nil || in.Query == "" {
			return ""
		}
		return toolCard("⚙ WebSearch", "", cName.Render("“"+in.Query+"”"), width)

	case "TodoWrite":
		var in struct {
			Todos []struct {
				Content    string `json:"content"`
				Status     string `json:"status"`
				ActiveForm string `json:"activeForm"`
			} `json:"todos"`
		}
		if json.Unmarshal(input, &in) != nil || len(in.Todos) == 0 {
			return ""
		}
		var lines []string
		for _, t := range in.Todos {
			mark := "[ ]"
			style := cDim
			switch t.Status {
			case "in_progress":
				mark = "[~]"
				style = cName
			case "completed":
				mark = "[x]"
				style = cTool
			}
			text := t.Content
			if t.Status == "in_progress" && t.ActiveForm != "" {
				text = t.ActiveForm
			}
			lines = append(lines, style.Render(mark+" "+text))
		}
		return toolCard("⚙ Todos", "", strings.Join(lines, "\n"), width)

	case "Task":
		var in struct {
			Description  string `json:"description"`
			SubagentType string `json:"subagent_type"`
			Prompt       string `json:"prompt"`
		}
		if json.Unmarshal(input, &in) != nil {
			return ""
		}
		head := in.Description
		if in.SubagentType != "" {
			head = in.SubagentType + " · " + head
		}
		body := head
		if in.Prompt != "" {
			body += "\n" + cDim.Render(trunc(in.Prompt, (width-4)*3))
		}
		return toolCard("⚙ Task", "", body, width)
	}
	return ""
}

// flattenToolResult turns the Content of a tool_result block into a plain
// string. The Anthropic API allows either a bare string or an array of typed
// content parts (currently just `{"type":"text","text":"..."}`). We accept
// both shapes and concatenate the text parts.
func flattenToolResult(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for i, p := range parts {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(p.Text)
		}
		return b.String()
	}
	// Last resort: drop the raw JSON in so something shows up.
	return string(raw)
}

// renderToolResult prints a green/red box with the tool's flattened output.
// The body is truncated to maxLines so a huge `cat` or build log doesn't
// dominate the transcript; a "… N more lines" trailer hints at the cut.
func renderToolResult(name, body string, isErr bool, width int) string {
	if width < 24 {
		width = 24
	}
	const maxLines = 20
	head := "↳ result"
	if name != "" {
		head = "↳ " + name + " result"
	}
	titleStyle := cTool
	if isErr {
		titleStyle = cErr
		head = "✗ " + name + " error"
	}
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	hidden := 0
	if len(lines) > maxLines {
		hidden = len(lines) - maxLines
		lines = lines[:maxLines]
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(head))
	b.WriteString("\n")
	for _, l := range lines {
		b.WriteString(cDim.Render(trunc(l, width-4)))
		b.WriteString("\n")
	}
	if hidden > 0 {
		b.WriteString(cDim.Render(fmt.Sprintf("… %d more lines", hidden)))
	}
	box := toolBox
	if isErr {
		box = toolBox.BorderForeground(colRed)
	}
	return box.Width(width - 2).Render(strings.TrimRight(b.String(), "\n"))
}

// toolCard wraps a typed tool card in the standard green double-line box.
// `head` is the bold title row, `note` is an optional dim subtitle below the
// title, and `body` is the typed content.
func toolCard(head, note, body string, width int) string {
	if width < 24 {
		width = 24
	}
	var b strings.Builder
	b.WriteString(cTool.Render(head))
	if note != "" {
		b.WriteString("  " + cDim.Render(note))
	}
	b.WriteString("\n")
	b.WriteString(body)
	return toolBox.Width(width - 2).Render(strings.TrimRight(b.String(), "\n"))
}

// compact collapses whitespace and caps to 200 chars, for the fallback
// tool card body where we dump raw JSON input.
func compact(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		return s[:197] + "…"
	}
	return s
}

// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

// Two independent paths surface the same tool call. The assistant event that
// announces the tool_use draws a card or a diff (stream.go), and so does the
// approval request the CLI raises just before it runs the tool (update.go). In
// ask mode both fire for every gated tool, so every diff was drawn twice.
//
// noteToolCard is the gate between them: the first path to see a tool call
// draws it, the second skips. Which one wins doesn't matter — they carry the
// same tool name and input — so neither side needs to know about the other.
//
// The key is the tool_use id. The CLI passes tool_use_id to the
// --permission-prompt-tool alongside tool_name and input (verified in the CLI
// 2.1.220 bundle), so both sides carry it. The input is NOT usable as a key: a
// hook or a permission rule can rewrite it before the approval request, so the
// two sides can disagree on the bytes while naming one call.
func (m *model) noteToolCard(id string) bool {
	if id == "" {
		return true // nothing to pair on (older CLI): draw it, as before
	}
	if m.shownTools == nil {
		m.shownTools = map[string]bool{}
	}
	if m.shownTools[id] {
		return false
	}
	m.shownTools[id] = true
	return true
}

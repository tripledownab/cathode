// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
)

// codexItem renders one conversation item. started says whether this is the
// opening event, which is the one that carries a tool item's content.
func (m *model) codexItem(f codexFrame, started bool) {
	var p struct {
		Item json.RawMessage `json:"item"`
	}
	if json.Unmarshal(f.Params, &p) != nil || len(p.Item) == 0 {
		return
	}
	var head struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	if json.Unmarshal(p.Item, &head) != nil {
		return
	}

	switch head.Type {
	case "userMessage":
		// Already in the transcript: sendTurn adds the typed text before the
		// turn opens, the same reason stream.go hides claude's echo.
		return
	case "agentMessage":
		if started {
			return // text arrives on completion
		}
		if t := strings.TrimSpace(head.Text); t != "" {
			m.add(entClaude, t)
		}
	case "reasoning":
		if started {
			return
		}
		if t := codexReasoningText(p.Item); t != "" {
			m.add(entThinking, t)
		}
	default:
		// A tool item. Its content is on the opening event, and the id pairs it
		// with the approval request that may follow, so noteToolCard keeps the
		// two from both drawing (toolcard.go).
		if !started {
			return
		}
		if head.ID != "" && !m.noteToolCard(head.ID) {
			return
		}
		m.addTool(head.Type, p.Item)
	}
}

// codexReasoningText flattens a reasoning item. The summary is what the
// interactive UI shows, so prefer it and fall back to the full content.
func codexReasoningText(raw json.RawMessage) string {
	var r struct {
		Summary []struct {
			Text string `json:"text"`
		} `json:"summary"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return ""
	}
	var b []string
	for _, s := range r.Summary {
		if t := strings.TrimSpace(s.Text); t != "" {
			b = append(b, t)
		}
	}
	if len(b) == 0 {
		for _, c := range r.Content {
			if t := strings.TrimSpace(c.Text); t != "" {
				b = append(b, t)
			}
		}
	}
	return strings.Join(b, "\n\n")
}

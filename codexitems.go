// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"path/filepath"
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
	case "fileChange":
		if !started {
			return
		}
		if head.ID != "" && !m.noteToolCard(head.ID) {
			return
		}
		if ds := codexFileDiffs(p.Item, m.agentCwd); len(ds) > 0 {
			m.addDiffs(ds)
			return
		}
		m.addTool(head.Type, p.Item)

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

// codexFileDiffs turns an item/fileChange into diff cards.
//
// The `diff` field means two different things depending on the kind, which is
// the whole reason this function exists rather than one assignment:
//
//	add     the new file's CONTENT
//	delete  the removed file's CONTENT
//	update  an already-computed unified hunk ("@@ -1,3 +1,3 @@ …")
//
// All three were confirmed against the live CLI. An update carries no copy of
// the whole file, so there is no before/after pair to build — it goes through
// fileDiff.unified, which both renderers accept because they parse unified
// text anyway.
func codexFileDiffs(raw json.RawMessage, root string) []fileDiff {
	var it struct {
		Changes []struct {
			Path string `json:"path"`
			Kind struct {
				Type string `json:"type"`
			} `json:"kind"`
			Diff string `json:"diff"`
		} `json:"changes"`
	}
	if json.Unmarshal(raw, &it) != nil {
		return nil
	}
	var out []fileDiff
	for _, c := range it.Changes {
		if c.Path == "" {
			continue
		}
		d := fileDiff{file: codexShortPath(c.Path, root)}
		switch c.Kind.Type {
		case "add":
			d.new = c.Diff
		case "delete":
			d.old = c.Diff
		case "update":
			d.unified = c.Diff
		default:
			continue // an unknown kind: a plain card says more than a blank diff
		}
		out = append(out, d)
	}
	return out
}

// codexShortPath trims the session's working root off a change path.
//
// codex reports absolute paths. The card title is more readable relative, and
// a screenshot of a session then carries no home directory. root is what the
// agent reported for the thread, NOT os.Getwd: those are equal by convention
// only, and a session rooted elsewhere would render every path in full.
// Falls back to the original when root is unknown or the path sits outside it.
func codexShortPath(p, root string) string {
	if root == "" {
		return p
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == "" || strings.HasPrefix(rel, "..") {
		return p
	}
	return rel
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

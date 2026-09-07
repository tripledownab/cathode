// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
)

// handleCodexEvent routes one app-server frame into the model, the way
// stream.go:handleEvent does for claude. Everything below `entry` is shared, so
// this file only decides which entry kind a frame becomes.
//
// Where a payload lives differs from claude in one way worth stating: for a
// tool item the content is on `item/started`, not on the completion and not on
// the approval request. So a command and a file change are drawn when they
// start, and their completion only updates status. Drawing on completion
// instead would leave a long command invisible while it runs, and drawing on
// the approval is not possible at all — that request carries ids and nothing
// else (see the approval work).
func (m *model) handleCodexEvent(f codexFrame) {
	switch f.Method {
	case "thread/started":
		m.noteCodexThread(f)
	case "turn/started":
		m.busy = true
	case "turn/completed", "turn/failed":
		m.noteCodexTurnEnd(f)
	case "item/started":
		m.codexItem(f, true)
	case "item/completed":
		m.codexItem(f, false)
	case "thread/tokenUsage/updated":
		m.noteCodexTokens(f)
	case "error", codexErrorMethod:
		var p struct {
			Message string `json:"message"`
			Error   struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(f.Params, &p)
		msg := p.Message
		if msg == "" {
			msg = p.Error.Message
		}
		if msg == "" {
			msg = "codex reported an error"
		}
		m.add(entError, "✗ "+msg)
	case codexClosedMethod:
		m.busy = false
		m.add(entInfo, "— session ended —")
	}
}

// noteCodexThread records the thread id. codex calls it a thread; cathode's
// session field holds it, so ctrl+r and the status row keep working unchanged.
func (m *model) noteCodexThread(f codexFrame) {
	var p struct {
		Thread struct {
			ID    string `json:"id"`
			Model string `json:"model"`
		} `json:"thread"`
	}
	if json.Unmarshal(f.Params, &p) != nil || p.Thread.ID == "" {
		return
	}
	m.session = p.Thread.ID
	// The resolved model is on the thread, not announced separately the way
	// claude's system/init reports it. Without this the session line renders a
	// bare separator where the model name belongs.
	if p.Thread.Model != "" {
		m.modelID = p.Thread.Model
	}
	m.add(entInfo, fmt.Sprintf("— thread %s · %s —", short(p.Thread.ID), m.modelID))
}

// noteCodexTurnEnd closes out a turn. There is no USD figure to report on a
// subscription, so the line carries what codex does report: how long it took.
func (m *model) noteCodexTurnEnd(f codexFrame) {
	m.busy = false
	var p struct {
		Turn struct {
			DurationMS int `json:"durationMs"`
			Error      *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
	}
	_ = json.Unmarshal(f.Params, &p)
	if p.Turn.Error != nil && p.Turn.Error.Message != "" {
		m.add(entError, "✗ "+p.Turn.Error.Message)
	}
	if p.Turn.DurationMS > 0 {
		m.add(entInfo, fmt.Sprintf("— done · %dms —", p.Turn.DurationMS))
		return
	}
	m.add(entInfo, "— done —")
}

// noteCodexTokens drives the context gauge.
//
// codex states modelContextWindow outright, which claude never does. cathode's
// -ctx flag and the auto-grow in observeCtx exist only because that number has
// to be guessed on claude; here the gauge can be exact, so the reported window
// wins over the flag.
func (m *model) noteCodexTokens(f codexFrame) {
	var p struct {
		TokenUsage struct {
			// inputTokens already counts the cached portion — the sibling
			// cachedInputTokens is a subset of it, not an addition, so adding
			// the two would roughly double the reported context.
			Total struct {
				InputTokens  int `json:"inputTokens"`
				OutputTokens int `json:"outputTokens"`
			} `json:"total"`
			ModelContextWindow int `json:"modelContextWindow"`
		} `json:"tokenUsage"`
	}
	if json.Unmarshal(f.Params, &p) != nil {
		return
	}
	if w := p.TokenUsage.ModelContextWindow; w > 0 {
		m.ctxLimit = w
	}
	m.outTokens = p.TokenUsage.Total.OutputTokens
	m.observeCtx(p.TokenUsage.Total.InputTokens)
}

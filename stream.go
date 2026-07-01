package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// handleEvent routes one parsed stream-json envelope into the model. Called
// from Update on every streamMsg; mutates m and rebuilds via the add helpers.
func (m *model) handleEvent(e Envelope) {
	switch e.Type {
	case "system":
		switch e.Subtype {
		case "init":
			m.session, m.modelID = e.Session, e.Model
			cwd, _ := os.Getwd()
			m.sessions.Touch(e.Session, e.Model, cwd, "", time.Now())
			m.add(entInfo, fmt.Sprintf("— session %s · %s —", short(e.Session), e.Model))
		case "hook_response":
			// Routine successful hooks (SessionStart, PreToolUse, …) fire constantly
			// and would flood the transcript, so stay silent on those. Surface only a
			// hook that failed or blocked — otherwise a tool silently not running is
			// a mystery.
			if e.ExitCode != 0 || (e.Outcome != "" && e.Outcome != "success") {
				msg := "⚙ hook " + e.HookName
				if e.Outcome != "" {
					msg += " " + e.Outcome
				}
				if s := strings.TrimSpace(e.Stderr); s != "" {
					msg += ": " + s
				}
				m.add(entError, msg)
			}
		case "status":
			// /compact streams these: "compacting" then a success/failed result.
			switch {
			case e.CompactResult == "success":
				// The exact post-compact context size isn't reported here — it
				// arrives with the next turn's usage. Zero the gauge now so it
				// reflects the freeing immediately instead of staying stuck at the
				// pre-compact level; observeCtx refines it on the next reply.
				m.ctxTokens = 0
				m.add(entInfo, "✓ context compacted")
			case e.CompactResult == "failed":
				m.add(entInfo, "✗ compact: "+e.CompactError)
			case e.Status == "compacting":
				m.add(entInfo, "→ compacting context…")
			}
		}
	case "assistant":
		if e.Message == nil {
			return
		}
		if u := e.Message.Usage; u != nil {
			// observeCtx records the live context size and auto-grows ctxLimit
			// past it (200K → 500K → 1M → 2M) so the long-context beta doesn't
			// peg the gauge. Same path the resume seed uses (see model.go).
			m.observeCtx(u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens)
			m.outTokens += u.OutputTokens
		}
		for _, b := range e.Message.Content {
			switch b.Type {
			case "text":
				if t := strings.TrimSpace(b.Text); t != "" {
					m.add(entClaude, t)
				}
			case "thinking":
				// Extended thinking — show it (dim) when present; many turns carry
				// an empty/again-signed block we just skip.
				if t := strings.TrimSpace(b.Thinking); t != "" {
					m.add(entThinking, t)
				}
			case "tool_use":
				// Remember the name so the tool_result event can label itself.
				if m.toolUses == nil {
					m.toolUses = map[string]string{}
				}
				if b.ID != "" {
					m.toolUses[b.ID] = b.Name
				}
				// AskUserQuestion is surfaced and answered through the approval
				// question picker (see question.go), so skip its raw tool card.
				if b.Name == askUserQuestionTool {
					continue
				}
				if ds, ok := diffsForTool(b.Name, b.Input); ok {
					m.addDiffs(ds)
				} else {
					m.addTool(b.Name, b.Input)
				}
			}
		}
	case "user":
		// claude folds tool outputs back into the conversation as a synthesised
		// user turn whose only content is tool_result blocks. We surface them
		// so the transcript shows what the tool actually returned.
		if e.Message == nil {
			return
		}
		for _, b := range e.Message.Content {
			if b.Type != "tool_result" {
				continue
			}
			name := m.toolUses[b.ToolUseID]
			// The AskUserQuestion result is our own deny-message (the injected
			// answer) coming back as an error — we already logged the answer via
			// the picker, so don't surface it as a failed tool.
			if name == askUserQuestionTool {
				continue
			}
			body := flattenToolResult(b.Content)
			m.addToolResult(name, body, b.IsError)
		}
	case "control_response":
		// The reply to our startup initialize carries the model list the
		// interactive /model menu shows; cache it for the picker. Other control
		// replies (set_model, etc.) are fire-and-forget and ignored here.
		if r := e.Response; r != nil && r.Subtype == "success" && r.Payload != nil {
			if len(r.Payload.Models) > 0 {
				m.models = r.Payload.Models
			}
			if len(r.Payload.Commands) > 0 {
				m.commands = r.Payload.Commands
			}
			if len(r.Payload.Agents) > 0 {
				m.agents = r.Payload.Agents
			}
		}
	case "result":
		m.busy = false
		m.lastCost = e.TotalCostUSD
		if e.IsError {
			m.add(entError, "✗ "+e.Result)
		}
		m.add(entInfo, fmt.Sprintf("— done · %.4f USD · %dms · %d turns —",
			e.TotalCostUSD, e.DurationMS, e.NumTurns))
		m.flushQueue()
	}
}

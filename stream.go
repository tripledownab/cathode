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
			// The server list only arrives here (not in the initialize handshake),
			// so cache it for the /mcp picker; keep the last non-empty snapshot the
			// way commands/agents are handled.
			if len(e.MCPServers) > 0 {
				m.mcpServers = e.MCPServers
			}
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
				m.stopCompacting()
				// The compact_boundary carrying the real numbers may have landed
				// first; if so, report those instead of the bare line.
				line := compactDoneText
				meta := m.compactMeta
				if meta != nil {
					line, m.compactMeta = compactSummary(*meta), nil
				}
				// Zeroing is a *placeholder* for the post-compact size, which this
				// status doesn't carry — observeCtx refines it on the next reply.
				// Don't apply it when a boundary already supplied the real figure
				// (noteCompaction), or we'd throw away the better number.
				if meta == nil || meta.PostTokens <= 0 {
					m.ctxTokens = 0
				}
				m.add(entInfo, line)
			case e.CompactResult == "failed":
				m.stopCompacting()
				m.add(entInfo, "✗ compact: "+e.CompactError)
			case e.Status == "compacting":
				// Raise the indeterminate progress bar (compact.go): compaction is
				// the one operation that can sit silent for a minute, and the
				// transcript line alone doesn't show it's still alive.
				m.startCompacting()
				m.add(entInfo, "→ compacting context…")
			}
		case "compact_boundary":
			// Emitted on a successful compaction with what it actually did. The
			// only real numbers the CLI gives us — everything during the run is
			// silent (see compact.go).
			if e.CompactMeta != nil {
				m.noteCompaction(*e.CompactMeta)
			}
		}
	case "assistant":
		if e.Message == nil {
			return
		}
		// A silent /mcp prime (mcp.go): swallow its text status — the picker opens
		// on the result instead, so the transcript stays clean.
		if m.mcpPriming {
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
		// Belt-and-braces: the turn is over, so the bar comes down even if the
		// compact_result line never arrived (errored turn, killed subprocess).
		m.stopCompacting()
		m.lastCost = e.TotalCostUSD
		// End of a silent /mcp prime: don't print the "— done —" line — just open
		// the picker now that init has cached the server list (mcp.go).
		if m.mcpPriming {
			m.mcpPriming = false
			m.openMCPPicker()
			return
		}
		if e.IsError {
			m.add(entError, "✗ "+e.Result)
		}
		m.add(entInfo, fmt.Sprintf("— done · %.4f USD · %dms · %d turns —",
			e.TotalCostUSD, e.DurationMS, e.NumTurns))
	}
}

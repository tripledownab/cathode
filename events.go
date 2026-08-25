// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import "encoding/json"

// Envelope is the common shape of every NDJSON line emitted by
// `claude ... --output-format stream-json`. We only declare the fields we
// actually render; unknown fields are ignored, so this stays resilient to
// schema additions.
//
// Observed line types:
//
//	{"type":"system","subtype":"init","session_id":"...","model":"...","tools":[...]}
//	{"type":"assistant","message":{"role":"assistant","content":[ ...blocks ]},"session_id":"..."}
//	{"type":"user","message":{"role":"user","content":[ {"type":"tool_result",...} ]}}
//	{"type":"result","subtype":"success","result":"...","total_cost_usd":0.01,"duration_ms":1234,"is_error":false,"session_id":"..."}
type Envelope struct {
	Type    string      `json:"type"`
	Subtype string      `json:"subtype"`
	Session string      `json:"session_id"`
	Model   string      `json:"model"`
	Message *APIMessage `json:"message"`

	// result-only fields
	Result       string  `json:"result"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	DurationMS   int     `json:"duration_ms"`
	NumTurns     int     `json:"num_turns"`
	IsError      bool    `json:"is_error"`

	// system/status fields (e.g. /compact progress)
	Status        string `json:"status"`
	CompactResult string `json:"compact_result"`
	CompactError  string `json:"compact_error"`

	// system/compact_boundary — the only place the CLI reports what a compaction
	// actually did. Present on success only; see CompactMeta.
	CompactMeta *CompactMeta `json:"compact_metadata"`

	// system/hook_response fields — we surface only failing/blocking hooks.
	HookName string `json:"hook_name"`
	ExitCode int    `json:"exit_code"`
	Outcome  string `json:"outcome"`
	Stderr   string `json:"stderr"`

	// system/init also reports the configured MCP servers and their status;
	// drives the /mcp picker (see mcp.go). Only present on the init line.
	MCPServers []MCPServerInfo `json:"mcp_servers"`

	// system/init reports the output style claude resolved ("default" when none
	// applies). It is the only feedback there is: claude ignores a style it
	// cannot find in silence, so /sysprompt has no other way to know its own
	// flag landed (outputstyle.go:initStyleNote).
	OutputStyle string `json:"output_style"`

	// control_response-only (e.g. the reply to our initialize handshake)
	Response *ControlResp `json:"response"`
}

// CompactMeta is the payload of a system/compact_boundary line: the real
// numbers behind a completed compaction. It is an *outcome* report, not
// progress — nothing at all is emitted between the "compacting" status and
// this (verified against the CLI's event-yield switch and on the wire).
//
// Observed live (v2.1.220), arriving just *after* the success status:
//
//	{"type":"system","subtype":"compact_boundary","compact_metadata":{
//	  "trigger":"manual","pre_tokens":22987,"post_tokens":1943,
//	  "cumulative_dropped_tokens":21044,"duration_ms":33334, ...}}
//
// Note post_tokens is populated here but is absent from the compact_boundary
// records persisted in claude's own session JSONL (0 of 234 locally) — the file
// is written at the boundary point, before the post-compact size is known. Read
// it from the stream, not from a replayed session.
//
// messages_summarized is documented by the CLI's mapper but doesn't always show
// up, so every consumer treats each field as optional.
type CompactMeta struct {
	Trigger            string `json:"trigger"` // "manual" (a /compact) or "auto" (context filled up)
	PreTokens          int    `json:"pre_tokens"`
	PostTokens         int    `json:"post_tokens"`
	DurationMS         int    `json:"duration_ms"`
	MessagesSummarized int    `json:"messages_summarized"`
}

// ControlResp wraps a control_response line — the reply to a control_request we
// sent (initialize, set_model, …). Subtype is "success" or "error"; Payload
// carries the initialize handshake's capability snapshot.
type ControlResp struct {
	Subtype   string       `json:"subtype"`
	RequestID string       `json:"request_id"`
	Error     string       `json:"error"`
	Payload   *InitPayload `json:"response"`
}

// InitPayload is the initialize handshake's inner payload. We pull the model
// list (for /model) plus the command and agent lists (for the palette and
// /agents). The CLI also reports output styles and account, which we ignore.
type InitPayload struct {
	Models   []ModelChoice `json:"models"`
	Commands []CommandInfo `json:"commands"`
	Agents   []AgentInfo   `json:"agents"`
}

// CommandInfo is one entry in the initialize command list — built-in slash
// commands, skills, and plugin commands all merged together (skills and plugin
// commands carry a "name:desc" the same as built-ins; plugin names are
// prefixed, e.g. "stripe:test-cards"). Drives the command palette.
type CommandInfo struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	ArgumentHint string `json:"argumentHint"`
}

// AgentInfo is one entry in the initialize agent list (built-in + plugin
// subagents). Listed by /agents; not directly invocable, so informational.
type AgentInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MCPServerInfo is one entry in the system:init mcp_servers list — a configured
// MCP server and its connection status ("connected", "needs-auth", "failed",
// "disabled", …). Drives the /mcp picker (mcp.go). The interactive server modal
// is a real-terminal feature the headless CLI can't render, so we reconstruct
// it from this list.
type MCPServerInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// ModelChoice is one entry in the initialize model list — the same set the
// interactive `claude` /model menu shows. Value is what set_model takes
// ("default", "opus[1m]", "sonnet", …); DisplayName/Description label the row.
type ModelChoice struct {
	Value       string `json:"value"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

// APIMessage mirrors the Anthropic message object carried inside assistant and
// user events.
type APIMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
	Usage   *Usage         `json:"usage,omitempty"`
}

// Usage is the per-message token breakdown reported by assistant envelopes.
// Context pressure = sum of all four / model context window.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// ContentBlock is one item in message.content. The same struct covers text,
// tool_use, and tool_result blocks; only the relevant fields are populated for
// each `Type`.
type ContentBlock struct {
	Type string `json:"type"`

	// type == "text"
	Text string `json:"text"`

	// type == "thinking" (extended thinking)
	Thinking string `json:"thinking"`

	// type == "tool_use"
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`

	// type == "tool_result"
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// parseEnvelope decodes a single NDJSON line. A nil error with a zero Type can
// happen for blank/keepalive lines; callers should skip those.
func parseEnvelope(line []byte) (Envelope, error) {
	var e Envelope
	err := json.Unmarshal(line, &e)
	return e, err
}

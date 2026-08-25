// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// The server list rides the system:init line (not the initialize handshake),
// so handleEvent must parse and cache it for the /mcp picker.
func TestMCPServersParsedFromInit(t *testing.T) {
	raw := `{"type":"system","subtype":"init","session_id":"","model":"opus",
	         "mcp_servers":[{"name":"stripe","status":"needs-auth"},
	                        {"name":"gh","status":"connected"}]}`
	var e Envelope
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("unmarshal init: %v", err)
	}
	// Session "" keeps sessions.Touch a no-op so a bare &model{} is safe here.
	m := &model{}
	m.handleEvent(e)
	if len(m.mcpServers) != 2 {
		t.Fatalf("want 2 cached servers, got %d", len(m.mcpServers))
	}
	if m.mcpServers[0].Name != "stripe" || m.mcpServers[0].Status != "needs-auth" {
		t.Errorf("first server mis-parsed: %+v", m.mcpServers[0])
	}
}

// Bare /mcp opens the picker once the server list is known; before that (no
// servers cached) it must NOT open an empty picker — it falls through to the
// forward path instead.
func TestMCPCommandOpensPickerWhenKnown(t *testing.T) {
	m := &model{mcpServers: []MCPServerInfo{{Name: "gh", Status: "connected"}}}
	nm, _ := m.mcpCommand("")
	if nm.picker == nil {
		t.Fatal("bare /mcp with a known server list should open the picker")
	}
	if nm.picker.kind != "mcp" {
		t.Errorf("picker kind = %q, want \"mcp\"", nm.picker.kind)
	}
	if len(nm.picker.items) != 1 || nm.picker.items[0].id != "gh" {
		t.Errorf("picker rows mis-built: %+v", nm.picker.items)
	}
}

// The first-invocation path: /mcp is typed before any turn, so the list isn't
// cached. A silent prime fetches it; when the init lands and the result closes
// the turn, the picker must open and the text echo / "— done —" line must be
// swallowed so the transcript stays clean.
func TestMCPPrimeOpensPickerOnResult(t *testing.T) {
	m := &model{mcpPriming: true}

	// The prime's own text status must not reach the transcript.
	m.handleEvent(Envelope{Type: "assistant", Message: &APIMessage{
		Content: []ContentBlock{{Type: "text", Text: "2 MCP server(s): ..."}},
	}})
	if len(m.entries) != 0 {
		t.Fatalf("primed /mcp echo leaked into transcript: %+v", m.entries)
	}

	// init caches the server list (Session "" keeps sessions.Touch a no-op).
	m.handleEvent(Envelope{Type: "system", Subtype: "init",
		MCPServers: []MCPServerInfo{{Name: "gh", Status: "connected"}}})

	// result closes the prime: picker opens, no "— done —" line, flag cleared.
	m.handleEvent(Envelope{Type: "result", Subtype: "success"})
	if m.mcpPriming {
		t.Error("mcpPriming should be cleared after the result")
	}
	if m.busy {
		t.Error("busy should be cleared after the result")
	}
	if m.picker == nil || m.picker.kind != "mcp" {
		t.Fatalf("picker should open on result, got %+v", m.picker)
	}
	for _, e := range m.entries {
		if strings.Contains(e.text, "done") {
			t.Errorf("prime should not print a done line, got %q", e.text)
		}
	}
}

// A prime that comes back with no servers configured opens no dialog — it says
// so plainly instead.
func TestMCPPrimeNoServers(t *testing.T) {
	m := &model{mcpPriming: true}
	m.handleEvent(Envelope{Type: "result", Subtype: "success"})
	if m.picker != nil {
		t.Error("no picker should open when no servers are configured")
	}
	if len(m.entries) != 1 || !strings.Contains(m.entries[0].text, "no MCP servers") {
		t.Errorf("expected a 'no MCP servers' info line, got %+v", m.entries)
	}
}

// Actions are contextual: a disabled server can be enabled; a live one can be
// reconnected or disabled. Each id is the exact "/mcp" argument to forward.
func TestMCPActionItemsContextual(t *testing.T) {
	enabled := mcpActionItems("gh", "connected")
	if len(enabled) != 2 || enabled[0].id != "reconnect gh" || enabled[1].id != "disable gh" {
		t.Errorf("connected server actions = %+v", enabled)
	}

	disabled := mcpActionItems("gh", "disabled")
	if len(disabled) == 0 || disabled[0].id != "enable gh" {
		t.Errorf("disabled server should offer enable first, got %+v", disabled)
	}
}

// needs-auth is surfaced with a hint that login is a terminal-only step, since
// the headless CLI can't run the OAuth flow.
func TestMCPStatusLabelAuthHint(t *testing.T) {
	if got := mcpStatusLabel("needs-auth"); !strings.Contains(got, "terminal") {
		t.Errorf("needs-auth label should point to a terminal, got %q", got)
	}
	if got := mcpStatusLabel("weird-new-status"); got != "weird-new-status" {
		t.Errorf("unknown status should pass through verbatim, got %q", got)
	}
}

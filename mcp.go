// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// mcpCommand implements /mcp. Bare /mcp opens the server picker; the rich
// interactive modal is a real-terminal feature the headless CLI can't render,
// so cathode reconstructs it from the server list. That list only rides the
// first turn's system:init (not the initialize handshake), so on the very first
// /mcp we don't have it yet — primeMCP fetches it silently and the picker opens
// when it lands (see handleEvent). With an argument ("reconnect all", "disable
// foo") we defer to claude's own /mcp, which owns those subcommands.
func (m *model) mcpCommand(arg string) (model, tea.Cmd) {
	if strings.TrimSpace(arg) == "" {
		if len(m.mcpServers) > 0 {
			m.openMCPPicker()
			return *m, nil
		}
		if !m.busy {
			return *m, m.primeMCP()
		}
	}
	return *m, m.sendMCP(arg)
}

// openMCPPicker shows the server picker, or an info line when nothing's
// configured — so /mcp never opens an empty dialog.
func (m *model) openMCPPicker() {
	if len(m.mcpServers) == 0 {
		m.add(entInfo, "no MCP servers configured")
		return
	}
	m.picker = newPicker("mcp", "MCP SERVERS", mcpItems(m.mcpServers), m.w, m.h)
}

// primeMCP fetches the server list by sending a bare /mcp to claude (a free,
// 0-turn client command that emits system:init with the list). It's sent raw —
// no user bubble, no history — and mcpPriming makes handleEvent swallow the text
// echo and open the picker on the result. Only used when the list isn't cached
// yet and we're idle; otherwise the picker opens straight away.
func (m *model) primeMCP() tea.Cmd {
	if err := m.engine.Send("/mcp"); err != nil {
		m.add(entError, "mcp: "+err.Error())
		return nil
	}
	m.mcpPriming = true
	m.busy = true
	return m.armSpinnerIfNeeded()
}

// sendMCP forwards a "/mcp <sub>" management turn to claude, guarding against
// injecting it mid-turn (that would read as steering, not a command). Shared by
// the bare-command fallback and the picker's action selection (see keys.go).
func (m *model) sendMCP(args string) tea.Cmd {
	line := "/mcp"
	if args = strings.TrimSpace(args); args != "" {
		line += " " + args
	}
	if m.busy {
		m.add(entInfo, "busy — try "+line+" after the current turn")
		return nil
	}
	return m.sendTurn(line)
}

// mcpItems renders the server list into picker rows (name + status). Selecting a
// row opens that server's action menu (see mcpActionItems).
func mcpItems(servers []MCPServerInfo) []pickerItem {
	items := make([]pickerItem, 0, len(servers))
	for _, s := range servers {
		items = append(items, pickerItem{id: s.Name, title: s.Name, subtitle: mcpStatusLabel(s.Status)})
	}
	return items
}

// mcpActionItems is the per-server action menu. Each id is the full /mcp
// argument ("reconnect <server>") so the dispatcher just forwards "/mcp "+id.
// Actions are contextual: a disabled server offers enable; anything else offers
// reconnect (retry connection/auth) and disable.
func mcpActionItems(server, status string) []pickerItem {
	if status == "disabled" {
		return []pickerItem{
			{id: "enable " + server, title: "enable", subtitle: "turn this server back on"},
			{id: "reconnect " + server, title: "reconnect", subtitle: "enable and retry the connection"},
		}
	}
	return []pickerItem{
		{id: "reconnect " + server, title: "reconnect", subtitle: "retry the connection / auth"},
		{id: "disable " + server, title: "disable", subtitle: "turn this server off"},
	}
}

// mcpStatusLabel humanises a server's raw status for the picker subtitle. Auth
// is called out because a needs-auth server can't be logged in headlessly — the
// OAuth handshake needs a real `claude mcp` / interactive `/mcp` session.
func mcpStatusLabel(status string) string {
	switch status {
	case "connected":
		return "● connected"
	case "needs-auth", "needs_auth":
		return "○ needs auth — run `claude mcp` in a terminal to log in"
	case "disabled":
		return "○ disabled"
	case "failed":
		return "✗ failed — try reconnect"
	case "pending", "connecting":
		return "… connecting"
	case "":
		return "status unknown"
	default:
		return status
	}
}

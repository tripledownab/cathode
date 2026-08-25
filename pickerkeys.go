// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
)

// handlePickerKey routes a keypress while a picker is open. A picker (session
// resume, palette, settings, question) is modal: every key goes through it
// until it returns a selection or cancels. The switch on the picker's kind is
// what a selection means — commit a setting, resume a session, answer a
// question. Split out of handleKey (keys.go), which keeps the global key table.
func (m model) handlePickerKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	// Keep the picker that handled the key: Enter closes it (next is nil), and
	// a checklist answer still has to be read out of it (pickermulti.go).
	cur := m.picker
	kind := cur.kind
	next, chosen := cur.Update(msg)
	m.picker = next
	// Live-preview pickers re-skin behind the dialog as the cursor moves;
	// Enter commits + persists, Esc reverts to the saved value.
	switch kind {
	case "header":
		switch {
		case chosen != "":
			m.commitHeaderStyle(chosen)
		case m.picker == nil:
			m.headerStyle = m.settings.Header
		default:
			if id := m.picker.focusedID(); id != "" {
				m.headerStyle = id
			}
		}
		// Switching to/from "off" toggles the animation; re-arm if it should
		// now run (no-op if a tick is already in flight).
		return m, m.armHeaderIfNeeded(), true
	case "banner":
		if chosen != "" {
			m.commitBanner(chosen)
		}
		return m, nil, true
	case "fps":
		if chosen != "" {
			m.commitFPS(chosen)
		}
		return m, m.armHeaderIfNeeded(), true
	case "theme":
		switch {
		case chosen != "":
			m.commitTheme(chosen)
		case m.picker == nil:
			applyTheme(m.settings.Theme)
			m.rerender()
		default:
			if id := m.picker.focusedID(); id != "" {
				applyTheme(id)
				m.rerender()
			}
		}
		return m, nil, true
	case "diff":
		if chosen != "" {
			m.commitDiff(chosen)
		}
		return m, nil, true
	case "sidebarpos":
		if chosen != "" {
			m.commitSidebarPos(chosen)
		}
		return m, nil, true
	case "bar":
		if chosen != "" {
			m.commitBar(chosen)
		}
		return m, nil, true
	case "sysprompt":
		// May return tea.Quit: applying it restarts into a resumed session
		// (sysprompt.go).
		if chosen != "" {
			return m, m.commitSysPrompt(chosen), true
		}
		return m, nil, true
	case "question":
		// Answering an AskUserQuestion: a pick records the answer (and may open
		// the next question); Esc (picker closed, no pick) dismisses it. A
		// multiSelect question answers with every marked row.
		switch {
		case chosen != "":
			return m, m.answerQuestion(cur.chosenIDs(chosen)), true
		case m.picker == nil:
			return m, m.cancelQuestion(), true
		}
		return m, nil, true
	}
	if chosen == "" {
		return m, nil, true
	}
	switch kind {
	case "sessions":
		return m, m.restartResuming(chosen), true
	case "slash":
		// Our commands run in-process; a claude / skill / plugin command from
		// the merged palette isn't ours, so forward it like a typed turn.
		if nm, cmd, handled := runSlash(&m, "/"+chosen); handled {
			return nm, cmd, true
		}
		cmd := m.sendTurn("/" + chosen)
		return m, cmd, true
	case "model":
		m.applyModel(chosen)
		return m, nil, true
	case "mcp":
		// A server was chosen — open its (status-dependent) action menu.
		status := ""
		for _, s := range m.mcpServers {
			if s.Name == chosen {
				status = s.Status
				break
			}
		}
		m.picker = newPicker("mcpaction", "MCP · "+chosen, mcpActionItems(chosen, status), m.w, m.h)
		return m, nil, true
	case "mcpaction":
		// chosen is the full /mcp argument, e.g. "reconnect foo".
		return m, m.sendMCP(chosen), true
	case "settings":
		// Top-level menu: open the chosen setting's picker, pre-positioned.
		switch chosen {
		case "header":
			p := newPicker("header", "HEADER", headerStyleItems(), m.w, m.h)
			p.setCursorTo(m.settings.Header)
			m.picker = p
		case "banner":
			p := newPicker("banner", "BANNER", bannerItems(), m.w, m.h)
			p.setCursorTo(m.settings.Banner)
			m.picker = p
		case "fps":
			p := newPicker("fps", "ANIMATION FPS", fpsItems(), m.w, m.h)
			p.setCursorTo(strconv.Itoa(m.settings.FPS))
			m.picker = p
		case "theme":
			p := newPicker("theme", "COLOR THEME", themeItems(), m.w, m.h)
			p.setCursorTo(m.settings.Theme)
			m.picker = p
		case "diff":
			p := newPicker("diff", "DIFF STYLE", diffItems(), m.w, m.h)
			p.setCursorTo(m.settings.Diff)
			m.picker = p
		case "sidebarpos":
			p := newPicker("sidebarpos", "SIDEBAR POSITION", sidebarPosItems(), m.w, m.h)
			p.setCursorTo(m.settings.Sidebar)
			m.picker = p
		case "bar":
			p := newPicker("bar", "COMPACT BAR", barItems(), m.w, m.h)
			p.setCursorTo(m.settings.Bar)
			m.picker = p
		case "sysprompt":
			p := newPicker("sysprompt", "EXTRA SYSTEM PROMPT", sysPromptItems(m.sysPromptEdited()), m.w, m.h)
			p.setCursorTo(sysPromptLabel(m.settings.SysPrompt))
			m.picker = p
		}
		return m, nil, true
	}
	return m, nil, true
}

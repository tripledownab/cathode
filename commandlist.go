// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// slashCommands is the table of commands cathode runs itself. Everything not
// listed here is forwarded to claude verbatim (runSlash, commands.go), so this
// is a table of overrides, not the set of commands the user can type.
func slashCommands() []slashCmd {
	return []slashCmd{
		{
			name: "clear",
			desc: "clear the transcript",
			exec: func(m *model, _ string) (model, tea.Cmd) {
				m.entries = m.entries[:0]
				m.rebuild()
				return *m, nil
			},
		},
		{
			name: "mode",
			desc: "set permission mode (plan|ask|build)",
			exec: func(m *model, arg string) (model, tea.Cmd) {
				arg = strings.TrimSpace(strings.ToLower(arg))
				if arg == "" {
					if m.mode == "bypass" {
						m.add(entInfo, "bypass mode: restart with -mode to switch")
					} else {
						m.mode = nextMode(m.mode)
						if err := m.engine.SetPermissionMode(modeToPermission(m.mode)); err != nil {
							m.add(entError, "mode toggle failed: "+err.Error())
						} else {
							m.add(entInfo, "→ mode: "+modeLabel(m.mode))
						}
					}
					return *m, nil
				}
				switch arg {
				case "plan", "ask", "build":
					m.mode = arg
					if err := m.engine.SetPermissionMode(modeToPermission(arg)); err != nil {
						m.add(entError, "mode set failed: "+err.Error())
					} else {
						m.add(entInfo, "→ mode: "+modeLabel(arg))
					}
				default:
					m.add(entError, "unknown mode: "+arg+" (plan|ask|build)")
				}
				return *m, nil
			},
		},
		{
			name: "mcp",
			desc: "manage MCP servers — status, reconnect/enable/disable",
			exec: func(m *model, arg string) (model, tea.Cmd) {
				return m.mcpCommand(arg)
			},
		},
		{
			name: "model",
			desc: "switch model (opus|sonnet|haiku|<id>)",
			exec: func(m *model, arg string) (model, tea.Cmd) {
				if strings.TrimSpace(arg) == "" {
					m.picker = newPicker("model", "SELECT MODEL", m.modelItems(), m.w, m.h)
					return *m, nil
				}
				m.applyModel(arg)
				return *m, nil
			},
		},
		{
			name: "mouse",
			desc: "toggle mouse capture — off (or shift+scroll) lets you select/copy text",
			exec: func(m *model, _ string) (model, tea.Cmd) {
				return *m, m.setMouseCapture(!m.mouse)
			},
		},
		{
			name: "settings",
			desc: "app settings — header animation, color theme (live preview)",
			exec: func(m *model, _ string) (model, tea.Cmd) {
				m.picker = newPicker("settings", "SETTINGS", m.settingsItems(), m.w, m.h)
				return *m, nil
			},
		},
		{
			name: "theme",
			desc: "pick a color theme (dracula, nord, …) with live preview",
			exec: func(m *model, _ string) (model, tea.Cmd) {
				p := newPicker("theme", "COLOR THEME", themeItems(), m.w, m.h)
				p.setCursorTo(m.settings.Theme)
				m.picker = p
				return *m, nil
			},
		},
		{
			name: "diff",
			desc: "diff card style (unified|split)",
			exec: func(m *model, arg string) (model, tea.Cmd) {
				switch strings.TrimSpace(strings.ToLower(arg)) {
				case diffUnified, diffSplit:
					m.commitDiff(strings.TrimSpace(strings.ToLower(arg)))
				case "":
					p := newPicker("diff", "DIFF STYLE", diffItems(), m.w, m.h)
					p.setCursorTo(m.settings.Diff)
					m.picker = p
				default:
					m.add(entError, "unknown diff style: "+arg+" (unified|split)")
				}
				return *m, nil
			},
		},
		{
			name: "sysprompt",
			desc: "toggle your standing instructions as claude's response style (on|off)",
			exec: func(m *model, arg string) (model, tea.Cmd) {
				switch id := strings.TrimSpace(strings.ToLower(arg)); id {
				case sysPromptOn, sysPromptOff:
					return *m, m.commitSysPrompt(id)
				case "":
					p := newPicker("sysprompt", "EXTRA SYSTEM PROMPT", sysPromptItems(m.sysPromptEdited()), m.w, m.h)
					p.setCursorTo(sysPromptLabel(m.settings.SysPrompt))
					m.picker = p
				default:
					m.add(entError, "unknown option: "+id+" (on|off)")
				}
				return *m, nil
			},
		},
		{
			name: "bar",
			desc: "/compact progress animation (comet|barber|pulse|scan|off)",
			exec: func(m *model, arg string) (model, tea.Cmd) {
				id := strings.TrimSpace(strings.ToLower(arg))
				if id == "" {
					p := newPicker("bar", "COMPACT BAR", barItems(), m.w, m.h)
					p.setCursorTo(m.settings.Bar)
					m.picker = p
					return *m, nil
				}
				if !validBarStyle(id) {
					m.add(entError, "unknown bar style: "+id+" (comet|barber|pulse|scan|off)")
					return *m, nil
				}
				m.commitBar(id)
				return *m, nil
			},
		},
		{
			name: "compact",
			desc: "summarise older turns to free up context",
			exec: func(m *model, _ string) (model, tea.Cmd) {
				// /compact is a built-in claude slash command, not a control
				// request — send it as a user turn so the CLI runs it. Progress
				// and the outcome arrive as system/status events (see handleEvent).
				if m.busy {
					m.add(entInfo, "busy — try /compact after the current turn")
					return *m, nil
				}
				if err := m.engine.Send("/compact"); err != nil {
					m.add(entError, "compact failed: "+err.Error())
					return *m, nil
				}
				m.busy = true
				return *m, m.armSpinnerIfNeeded()
			},
		},
		{
			name: "commands",
			desc: "browse all commands — built-in, skills, plugins (also ctrl+t)",
			exec: func(m *model, _ string) (model, tea.Cmd) {
				m.picker = newPicker("slash", "COMMANDS", m.paletteItems(), m.w, m.h)
				return *m, nil
			},
		},
		{
			name: "agents",
			desc: "list available subagents (built-in + plugin)",
			exec: func(m *model, _ string) (model, tea.Cmd) {
				if len(m.agents) == 0 {
					m.add(entInfo, "no agents reported yet (waiting on the initialize handshake)")
					return *m, nil
				}
				var b strings.Builder
				b.WriteString("available agents:\n")
				for _, a := range m.agents {
					b.WriteString(fmt.Sprintf("  %-22s %s\n", a.Name, a.Description))
				}
				m.add(entInfo, strings.TrimRight(b.String(), "\n"))
				return *m, nil
			},
		},
		{
			name: "sessions",
			desc: "resume a previous session",
			exec: func(m *model, _ string) (model, tea.Cmd) {
				cwd, _ := os.Getwd()
				m.picker = newPicker("sessions", "RESUME SESSION", sessionItems(m.sessions, cwd), m.w, m.h)
				return *m, nil
			},
		},
		{
			name: "cwd",
			desc: "show working directory",
			exec: func(m *model, _ string) (model, tea.Cmd) {
				wd, err := os.Getwd()
				if err != nil {
					m.add(entError, "cwd: "+err.Error())
				} else {
					m.add(entInfo, "cwd: "+wd)
				}
				return *m, nil
			},
		},
		{
			name: "sidebar",
			desc: "toggle the BBS info rail (or /sidebar left|right)",
			exec: func(m *model, arg string) (model, tea.Cmd) {
				// "/sidebar left|right" sets the side (and shows it); bare toggles.
				switch strings.TrimSpace(strings.ToLower(arg)) {
				case sidebarLeft, sidebarRight:
					if !m.sidebar {
						m.sidebar = true
						m.resizeViewport()
					}
					m.commitSidebarPos(strings.TrimSpace(strings.ToLower(arg)))
					return *m, nil
				case "":
				default:
					m.add(entError, "usage: /sidebar [left|right]")
					return *m, nil
				}
				m.sidebar = !m.sidebar
				m.resizeViewport()
				m.rebuild()
				if m.sidebar && m.w < sidebarMinWidth {
					m.add(entInfo, fmt.Sprintf("sidebar needs ≥%d cols (terminal is %d); will appear when widened", sidebarMinWidth, m.w))
				} else if m.sidebar {
					m.add(entInfo, "sidebar: on")
				} else {
					m.add(entInfo, "sidebar: off")
				}
				return *m, nil
			},
		},
		{
			name: "help",
			desc: "show keybindings and commands",
			exec: func(m *model, _ string) (model, tea.Cmd) {
				m.help = true
				return *m, nil
			},
		},
		{
			name: "quit",
			desc: "exit cathode",
			exec: func(m *model, _ string) (model, tea.Cmd) {
				// Quit; the subprocess is closed in main after Run() (Engine.Close).
				return *m, tea.Quit
			},
		},
	}
}

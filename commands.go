package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// slashCmd is a typed-in command, e.g. "/clear" or "/mode plan". The exec
// receives the model and any argument string (everything after the command
// name), and returns the next tea.Cmd plus an updated model. Returning a
// model with .picker set opens that picker; returning tea.Quit exits.
type slashCmd struct {
	name string
	desc string
	exec func(m *model, arg string) (model, tea.Cmd)
}

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
			desc: "toggle mouse capture — off lets you select/copy text",
			exec: func(m *model, _ string) (model, tea.Cmd) {
				m.mouse = !m.mouse
				if m.mouse {
					m.add(entInfo, "→ mouse: ON — wheel scrolls the transcript")
					return *m, tea.EnableMouseCellMotion
				}
				m.add(entInfo, "→ mouse: OFF — drag to select/copy · wheel or ↑/↓ scrolls · ctrl+↑/↓ history")
				return *m, tea.DisableMouse
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

// runSlash dispatches "/name [arg]" against our in-process command table.
// Returns handled=true when a command ran; handled=false when the line isn't a
// slash command OR isn't one of ours — in the latter case the caller forwards
// it to claude verbatim, so claude's built-in / custom / plugin slash commands
// still work (we don't reject what we don't recognize).
func runSlash(m *model, line string) (model, tea.Cmd, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "/") {
		return *m, nil, false
	}
	rest := strings.TrimPrefix(line, "/")
	name, arg, _ := strings.Cut(rest, " ")
	name = strings.ToLower(name)
	for _, c := range slashCommands() {
		if c.name == name {
			nm, cmd := c.exec(m, arg)
			return nm, cmd, true
		}
	}
	return *m, nil, false
}

// slashItems projects the slash command table into picker rows.
func slashItems() []pickerItem {
	cmds := slashCommands()
	items := make([]pickerItem, 0, len(cmds))
	for _, c := range cmds {
		items = append(items, pickerItem{id: c.name, title: "/" + c.name, subtitle: c.desc})
	}
	sort.SliceStable(items, func(a, b int) bool { return items[a].title < items[b].title })
	return items
}

// paletteItems is the command palette's rows: our in-process commands plus the
// ones claude reports from the initialize handshake (built-ins, skills, and
// plugin commands), deduped by name with ours winning — ours run locally, the
// rest are forwarded to claude on select.
func (m *model) paletteItems() []pickerItem {
	items := slashItems()
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		seen[it.id] = true
	}
	for _, c := range m.commands {
		if c.Name == "" || seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		items = append(items, pickerItem{id: c.Name, title: "/" + c.Name, subtitle: c.Description})
	}
	sort.SliceStable(items, func(a, b int) bool { return items[a].title < items[b].title })
	return items
}

// helpModalView is the boxed, centered version of the help text. Rendered by
// View() through lipgloss.Place so it looks like a floating modal.
func helpModalView(termW, termH int) string {
	w := termW - 8
	if w < 48 {
		w = 48
	}
	if w > 78 {
		w = 78
	}
	body := dTitle.Render(" HELP ") + "\n" + helpText() + "\n" +
		cDim.Render("  [esc / ?] close")
	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colCyan).
		Padding(0, 1).
		Width(w)
	return box.Render(body)
}

// helpText is what /help prints into the transcript.
func helpText() string {
	cmds := slashCommands()
	sort.SliceStable(cmds, func(a, b int) bool { return cmds[a].name < cmds[b].name })
	var b strings.Builder
	b.WriteString("keybindings:\n")
	b.WriteString("  enter         send  ·  alt+enter / ctrl+j / \\↵  insert a line break\n")
	b.WriteString("  shift+tab     cycle mode (plan → ask → build)\n")
	b.WriteString("  ctrl+r        resume a session\n")
	b.WriteString("  ctrl+t        slash command palette\n")
	b.WriteString("  ctrl+g        toggle the info sidebar (or /sidebar)\n")
	b.WriteString("  ?             open this help modal\n")
	b.WriteString("  ↑ / ↓         history · cursor between lines (multi-line) · scroll (mouse off)\n")
	b.WriteString("  ctrl+↑ / ↓    prompt history (always)\n")
	b.WriteString("  esc / ctrl+c  quit\n")
	b.WriteString("commands:\n")
	for _, c := range cmds {
		b.WriteString(fmt.Sprintf("  /%-10s %s\n", c.name, c.desc))
	}
	b.WriteString("  any other /command is forwarded to claude (custom & plugin commands)\n")
	return strings.TrimRight(b.String(), "\n")
}

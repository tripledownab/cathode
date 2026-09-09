// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// slashCmd is a typed-in command, e.g. "/clear" or "/mode plan". The exec
// receives the model and any argument string (everything after the command
// name), and returns the next tea.Cmd plus an updated model. Returning a
// model with .picker set opens that picker; returning tea.Quit exits.
type slashCmd struct {
	name string
	desc string
	// only lists the backends this command works on. Empty means every one.
	//
	// A command that depends on something one backend has — claude's output
	// styles, its MCP server list — is not merely useless elsewhere: offered
	// and selected, it either does nothing or gets forwarded to the agent as a
	// prompt, which is worse than not being there. Declaring it here keeps the
	// palette, the help text and the dispatcher agreeing, because all three ask
	// this one question.
	only []string
	exec func(m *model, arg string) (model, tea.Cmd)
}

// availableOn reports whether this command works on the given backend.
func (c slashCmd) availableOn(backend string) bool {
	if len(c.only) == 0 {
		return true
	}
	for _, b := range c.only {
		if b == sessionBackend(backend) {
			return true
		}
	}
	return false
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
		if c.name != name {
			continue
		}
		if !c.availableOn(m.backend) {
			// Handled, not forwarded. Falling through would send "/sysprompt"
			// to the agent as a prompt, which reads as the command silently
			// doing something odd rather than not existing here.
			m.add(entInfo, "/"+c.name+" is not available on the "+agentName(m.backend)+" backend")
			return *m, nil, true
		}
		nm, cmd := c.exec(m, arg)
		return nm, cmd, true
	}
	return *m, nil, false
}

// slashItems projects the slash command table into picker rows, omitting the
// commands this backend cannot run.
func slashItems(backend string) []pickerItem {
	cmds := slashCommands()
	items := make([]pickerItem, 0, len(cmds))
	for _, c := range cmds {
		if !c.availableOn(backend) {
			continue
		}
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
	items := slashItems(m.backend)
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

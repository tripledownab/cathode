package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// modelItems is the row set for the /model picker. It prefers the live list
// from the initialize handshake (which mirrors the interactive /model menu,
// including the 1M-context entries); until that lands — or if it failed — it
// falls back to the three standard aliases.
func (m *model) modelItems() []pickerItem {
	if len(m.models) == 0 {
		return fallbackModelItems()
	}
	items := make([]pickerItem, 0, len(m.models))
	for _, mc := range m.models {
		items = append(items, pickerItem{id: mc.Value, title: mc.DisplayName, subtitle: mc.Description})
	}
	return items
}

// fallbackModelItems is the static list used before the initialize handshake
// replies (or if it never does). Aliases, so they resolve to whatever the
// subscription's current generation maps to.
func fallbackModelItems() []pickerItem {
	return []pickerItem{
		{id: "opus", title: "opus", subtitle: "most capable — deep reasoning, big refactors"},
		{id: "sonnet", title: "sonnet", subtitle: "balanced — the everyday default"},
		{id: "haiku", title: "haiku", subtitle: "fastest — quick edits, low latency"},
	}
}

// requestModels runs the initialize handshake so the model list is cached
// before the user opens /model. The reply arrives via the stream as a
// control_response (see handleEvent). Wired into model.Init().
func requestModels(e *Engine) tea.Cmd {
	return func() tea.Msg {
		_ = e.Initialize()
		return nil
	}
}

// applyModel switches the live model and records it for the status/sidebar. The
// change rides a set_model control request, so it takes effect on the next turn
// without restarting the subprocess (see Engine.SetModel).
func (m *model) applyModel(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if err := m.engine.SetModel(name); err != nil {
		m.add(entError, "model switch failed: "+err.Error())
		return
	}
	// Prefer the menu's display name for the status/sidebar when we know it, so
	// "default" / "opus[1m]" read as "Default (recommended)" / "Opus".
	label := name
	for _, mc := range m.models {
		if mc.Value == name {
			label = mc.DisplayName
			break
		}
	}
	m.modelID = label
	m.add(entInfo, "→ model: "+label)
}

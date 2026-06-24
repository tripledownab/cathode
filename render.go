package main

import "strings"

// rebuild walks m.entries and writes the rendered transcript into the
// viewport. Called on every new entry and on resize so markdown reflows.
func (m *model) rebuild() {
	if !m.ready {
		return
	}
	var b strings.Builder
	for i, e := range m.entries {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(m.renderEntry(e))
		b.WriteString("\n")
	}
	m.vp.SetContent(b.String())
	// Only chase the bottom while following; if the user has scrolled up to
	// read back, leave their position put (see scroll.go).
	if m.follow {
		m.vp.GotoBottom()
	}
}

// renderEntry dispatches a single transcript item to its renderer. Kept out
// of rebuild() so the big switch doesn't drown the layout loop.
func (m *model) renderEntry(e entry) string {
	switch e.kind {
	case entUser:
		return userBox.Render(cYou.Render(ornBullet+" "+studly("you")) + "\n" + e.text)
	case entClaude:
		body := e.text
		if m.md != nil {
			if out, err := m.md.Render(e.text); err == nil {
				body = strings.TrimRight(out, "\n")
			}
		}
		return cName.Render(ornBullet+" "+studly("claude")) + "\n" + body
	case entTool:
		// Typed renderer when we still have the structured input; falls back
		// to the legacy "name\nJSON" format otherwise.
		if e.toolName != "" {
			if card := renderTool(e.toolName, e.toolInput, m.vp.Width); card != "" {
				return card
			}
			card := cTool.Render("⚙ "+e.toolName) + "\n" + cDim.Render(compact(string(e.toolInput)))
			return toolBox.Width(m.vp.Width - 2).Render(strings.TrimRight(card, "\n"))
		}
		parts := strings.SplitN(e.text, "\n", 2)
		name := parts[0]
		arg := ""
		if len(parts) > 1 {
			arg = cDim.Render(parts[1])
		}
		card := cTool.Render("⚙ "+name) + "\n" + arg
		return toolBox.Render(strings.TrimRight(card, "\n"))
	case entToolResult:
		return renderToolResult(e.toolName, e.toolResult, e.toolError, m.vp.Width)
	case entDiff:
		parts := make([]string, 0, len(e.diffs))
		for _, d := range e.diffs {
			parts = append(parts, renderDiff(d.file, d.old, d.new, m.vp.Width))
		}
		return strings.Join(parts, "\n")
	case entInfo:
		return cDim.Render(e.text)
	case entError:
		return cErr.Render(e.text)
	}
	return ""
}

// resizeViewport sets m.vp.Width/Height based on the current window size,
// the sidebar flag, and the pending-tray height. Call after anything that
// changes those (window resize, sidebar toggle, queue mutation).
func (m *model) resizeViewport() {
	vpH := m.h - 6 // banner(3) + divider(1) + prompt(1) + status(1)
	// Account for the pending tray when visible.
	if n := len(m.queue); n > 0 {
		const maxShown = 5
		extra := 1 + minInt(n, maxShown)
		if n > maxShown {
			extra++
		}
		vpH -= extra
	}
	if vpH < 1 {
		vpH = 1
	}
	vpW := m.w - 1 // reserve 1 col for the scrollbar
	if m.sidebar && m.w >= sidebarMinWidth {
		vpW -= sidebarWidth
	}
	if vpW < 1 {
		vpW = 1
	}
	if !m.ready {
		m.vp = newTranscriptViewport(vpW, vpH)
		m.ready = true
	} else {
		m.vp.Width, m.vp.Height = vpW, vpH
	}
}

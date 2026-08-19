package main

import (
	"strings"

	"github.com/rivo/uniseg"
)

// rebuild writes the rendered transcript into the viewport. Each entry is
// rendered once and appended to m.content (a retained buffer), so the common
// case — one new entry — renders just that tail and appends O(new) bytes,
// instead of re-running Glamour over the whole transcript AND re-joining every
// render into a fresh O(total) string each message. A full re-render runs only
// when the wrap width changed or entries were removed (e.g. /clear), since then
// every render must reflow.
func (m *model) rebuild() {
	if !m.ready {
		return
	}
	if m.content == nil {
		m.content = &strings.Builder{}
	}
	if m.cacheWidth != m.vp.Width || m.renderedCount > len(m.entries) {
		m.content.Reset()
		m.renderedCount = 0
		m.entryLine, m.lineCount = m.entryLine[:0], 0
		for _, e := range m.entries {
			m.appendEntry(linkify(m.renderEntry(e)))
		}
		m.cacheWidth = m.vp.Width
	} else {
		for i := m.renderedCount; i < len(m.entries); i++ {
			m.appendEntry(linkify(m.renderEntry(m.entries[i])))
		}
	}
	// Builder.String() hands the viewport its backing bytes without copying;
	// appends after this only ever write past the bytes the viewport split on,
	// so prior lines stay valid (the next rebuild re-splits anyway).
	m.vp.SetContent(m.content.String())
	// Content changed, so the memoized frame body (view.go) is stale.
	m.contentVer++
	// Only chase the bottom while following; if the user has scrolled up to
	// read back, leave their position put (see scroll.go).
	if m.follow {
		m.vp.GotoBottom()
	}
}

// appendEntry writes one entry's render to the content buffer: a blank line
// before every entry after the first, then the render and a terminating
// newline. This reproduces the old "join with \n\n plus a trailing \n" layout
// while only touching the new tail.
// It also records where the entry starts, which is what makes an entry index
// addressable as a scroll offset (jump.go). The count is exact because every
// renderer wraps to m.vp.Width, so one content line is one viewport line.
func (m *model) appendEntry(s string) {
	if m.renderedCount > 0 {
		m.content.WriteString("\n")
		m.lineCount++
	}
	m.entryLine = append(m.entryLine, m.lineCount)
	m.content.WriteString(s)
	m.content.WriteString("\n")
	m.lineCount += strings.Count(s, "\n") + 1
	m.renderedCount++
}

// rerender drops the per-width render cache and rebuilds from scratch. Needed
// when something other than width changes how entries render — a theme swap or
// a diff-style toggle — since rebuild() otherwise reuses the cached renders.
func (m *model) rerender() {
	m.cacheWidth = -1
	m.rebuild()
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
	case entThinking:
		// Extended thinking: dim + italic so it reads as the model's scratch work,
		// visually subordinate to the actual reply.
		return cDim.Render(ornBullet+" "+studly("thinking")) + "\n" + cDim.Italic(true).Render(e.text)
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
			parts = append(parts, renderDiffFor(m.settings.Diff, d.file, d.old, d.new, m.vp.Width))
		}
		return strings.Join(parts, "\n")
	case entInfo:
		return cDim.Render(e.text)
	case entError:
		return cErr.Render(e.text)
	}
	return ""
}

// resizeViewport sets m.vp.Width/Height based on the current window size and
// the sidebar flag. Call after anything that changes those (window resize,
// sidebar toggle, prompt height change).
func (m *model) resizeViewport() {
	// banner(3) + divider(1) + status(1) + prompt + the compact bar when it's up
	vpH := m.h - 5 - m.promptRows() - m.compactRows()
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

// maxPromptRows caps how tall the prompt grows before it scrolls internally.
const maxPromptRows = 8

// promptRows is how many terminal rows the prompt occupies: 1 for the approval
// bar, else the textarea's current height (1..maxPromptRows).
func (m *model) promptRows() int {
	if m.pending != nil {
		return 1
	}
	h := m.input.Height()
	if h < 1 {
		h = 1
	}
	return h
}

// syncPromptHeight grows/shrinks the input to fit its content — hard line
// breaks AND soft-wrapped rows (promptwrap.go), so a long line in a narrow
// window stays fully visible instead of scrolling out of sight — capped at
// maxPromptRows, resizing the transcript viewport when the row count changes.
// Called once per Update after the input has handled the message.
//
// The textarea scrolls its internal viewport while it is still the OLD height
// (its Update runs before this), and SetHeight doesn't re-anchor — so after
// growing we must reset that scroll or only the last row(s) stay visible.
// reanchorPrompt does that whenever the content fits the (new) height and a
// scroll could have happened: the height just changed, or we've come back
// under the cap.
func (m *model) syncPromptHeight() {
	if m.pending != nil {
		return // the approval bar replaces the prompt
	}
	rows := promptVisualRows(m.input.Value(), m.promptInnerWidth())
	want := rows
	if want > maxPromptRows {
		want = maxPromptRows
	}
	changed := want != m.input.Height()
	if changed {
		m.input.SetHeight(want)
		m.resizeViewport()
	}
	if rows <= maxPromptRows && (changed || m.lastPromptRows > maxPromptRows) {
		m.reanchorPrompt()
	}
	m.lastPromptRows = rows
}

// reanchorPrompt scrolls the textarea's internal viewport back to the top —
// the only public route is rebuilding the value (SetValue → Reset → GotoTop) —
// and then restores the cursor: walk up to its hard line, then set the column.
func (m *model) reanchorPrompt() {
	row := m.input.Line()
	li := m.input.LineInfo()
	col := li.StartColumn + li.CharOffset
	m.input.SetValue(m.input.Value()) // resets scroll; cursor lands at the end
	for i := 0; i < 1000 && m.input.Line() > row; i++ {
		m.input.CursorUp()
	}
	m.input.SetCursor(col)
}

// promptInnerWidth is the width the textarea wraps text at: the total width we
// gave it minus the cells its prompt string occupies (no line numbers, no
// frame — mirrors textarea.SetWidth's reservation math for our config).
func (m *model) promptInnerWidth() int {
	inner := m.promptW - uniseg.StringWidth(m.input.Prompt)
	if inner < 1 {
		inner = 1
	}
	return inner
}

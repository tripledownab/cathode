package main

import (
	"fmt"
	"strconv"
	"strings"

	udiff "github.com/aymanbagabas/go-udiff"
	"github.com/charmbracelet/lipgloss"
)

// Diff display styles (persisted as settings.Diff).
const (
	diffUnified = "unified" // single column with a +/- gutter (renderDiff)
	diffSplit   = "split"   // side-by-side old | new (renderDiffSplit)
)

// splitMinWidth is the narrowest terminal where two columns stay legible. Below
// it — or in unified mode — we render the single-column diff instead.
const splitMinWidth = 80

type diffOption struct{ id, label, desc string }

var diffOptions = []diffOption{
	{diffUnified, "unified", "one column with a +/- line gutter (default)"},
	{diffSplit, "split", "side-by-side old │ new (unified when too narrow)"},
}

func diffLabel(id string) string {
	for _, o := range diffOptions {
		if o.id == id {
			return o.label
		}
	}
	return id
}

func diffItems() []pickerItem {
	items := make([]pickerItem, 0, len(diffOptions))
	for _, o := range diffOptions {
		items = append(items, pickerItem{id: o.id, title: o.label, subtitle: o.desc})
	}
	return items
}

// commitDiff applies the chosen diff style, re-renders the transcript's existing
// diff cards in it, and persists the choice.
func (m *model) commitDiff(id string) {
	m.settings.Diff = id
	saveSettings(m.settings)
	m.rerender()
	m.add(entInfo, "→ diff: "+diffLabel(id))
}

// renderDiffFor dispatches to the configured diff style, falling back to the
// unified card when split is selected but the terminal is too narrow for it.
func renderDiffFor(style, filename, oldText, newText string, width int) string {
	if style == diffSplit && width >= splitMinWidth {
		return renderDiffSplit(filename, oldText, newText, width)
	}
	return renderDiff(filename, oldText, newText, width)
}

// renderDiffSplit builds a side-by-side diff card: deletions (with old line
// numbers) on the left, additions (new line numbers) on the right, context on
// both. A change is shown as a removed line beside its added line; unpaired
// adds/dels leave the opposite column blank.
func renderDiffSplit(filename, oldText, newText string, width int) string {
	u := udiff.Unified("a/"+filename, "b/"+filename, oldText, newText)
	if strings.TrimSpace(u) == "" {
		return dBox.Width(width - 2).Render(dTitle.Render(" "+filename+" ") + "\n" + dCtx.Render("(no changes)"))
	}

	innerW := width - 4 // inside the box border + padding
	if innerW < 24 {
		innerW = 24
	}
	const numW = 4           // line-number gutter per side
	half := (innerW - 3) / 2 // 3 = the visible " │ " column separator
	colW := half - numW - 1  // text width per side (after the number + a space)
	if colW < 4 {
		colW = 4
	}

	var body strings.Builder
	added, removed := 0, 0
	oldLn, newLn := 0, 0
	var dels, adds []string // a pending change run, flushed on context / hunk / end

	flush := func() {
		n := len(dels)
		if len(adds) > n {
			n = len(adds)
		}
		for i := 0; i < n; i++ {
			left := blankCell(numW, colW)
			if i < len(dels) {
				left = sideCell(strconv.Itoa(oldLn), dels[i], numW, colW, dDel)
				oldLn++
			}
			right := blankCell(numW, colW)
			if i < len(adds) {
				right = sideCell(strconv.Itoa(newLn), adds[i], numW, colW, dAdd)
				newLn++
			}
			body.WriteString(left + dGutter.Render(" │ ") + right + "\n")
		}
		dels, adds = dels[:0], adds[:0]
	}

	for _, raw := range strings.Split(u, "\n") {
		switch {
		case strings.HasPrefix(raw, "+++"), strings.HasPrefix(raw, "---"):
			continue
		case strings.HasPrefix(raw, "@@"):
			flush()
			oldLn, newLn = parseHunk(raw)
			body.WriteString(dHunk.Render(trunc(raw, innerW)) + "\n")
		case strings.HasPrefix(raw, "-"):
			removed++
			dels = append(dels, raw[1:])
		case strings.HasPrefix(raw, "+"):
			added++
			adds = append(adds, raw[1:])
		case strings.HasPrefix(raw, " "):
			flush()
			left := sideCell(strconv.Itoa(oldLn), raw[1:], numW, colW, dCtx)
			right := sideCell(strconv.Itoa(newLn), raw[1:], numW, colW, dCtx)
			oldLn++
			newLn++
			body.WriteString(left + dGutter.Render(" │ ") + right + "\n")
		}
	}
	flush()

	title := fmt.Sprintf(" %s  %s %s ", filename,
		dAdd.Render("+"+strconv.Itoa(added)), dDel.Render("-"+strconv.Itoa(removed)))
	return dBox.Width(width - 2).Render(
		dTitle.Render(title) + "\n" + strings.TrimRight(body.String(), "\n"))
}

// sideCell renders one column: a right-aligned line-number gutter, then the
// tab-expanded content clamped and padded to exactly colW so the divider lines
// up across rows. style colors the content (red dels / green adds / dim context).
func sideCell(num, text string, numW, colW int, style lipgloss.Style) string {
	return dGutter.Render(fmt.Sprintf("%*s ", numW, num)) + style.Render(padR(tabs(text), colW))
}

// blankCell is an empty column of the same visible width as a sideCell, used for
// the side with no counterpart in an unbalanced change run.
func blankCell(numW, colW int) string {
	return strings.Repeat(" ", numW+1+colW)
}

// padR clamps s to w cells (via trunc) then right-pads with spaces to exactly w.
func padR(s string, w int) string {
	s = trunc(s, w)
	if n := w - len([]rune(s)); n > 0 {
		s += strings.Repeat(" ", n)
	}
	return s
}

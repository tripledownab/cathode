package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	sidebarWidth    = 32
	sidebarMinWidth = 72 // sidebar(32) + scrollbar(1) + transcript ≥39
)

var sidebarBox lipgloss.Style

// buildSidebarStyles rebuilds the sidebar box from the active palette. Called
// from buildStyles (theme.go) on startup and every theme change.
func buildSidebarStyles() {
	sidebarBox = lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder(), false, true, false, false).
		BorderForeground(colCyan).
		Padding(0, 1)
}

// bbsSidebar renders the left info rail at a fixed width. Empty values render
// as "—" so columns stay aligned. Each row is truncated to the content area so
// nothing wraps to a second visual line, which would inflate the sidebar's
// height and push the banner off the top via JoinHorizontal.
func bbsSidebar(height int, mode, session, modelID, cwd string, cost float64, turns int) string {
	if height < 1 {
		height = 1
	}
	// Content area = width − 2 (Padding(0,1)) − 1 (right border).
	const contentArea = sidebarWidth - 3
	dash := func(s string) string {
		if strings.TrimSpace(s) == "" {
			return "—"
		}
		return s
	}
	tail := func(s string, w int) string {
		if w < 1 {
			w = 1
		}
		r := []rune(s)
		if len(r) <= w {
			return s
		}
		if w == 1 {
			return "…"
		}
		return "…" + string(r[len(r)-(w-1):])
	}
	label := func(s string) string { return cDim.Render(leet(s)) }
	val := func(s string) string { return hdrSub.Render(s) }
	// kv renders a "LABEL  value" row, truncating value to fit. We compute
	// the budget against the *display* (post-leet) length, since that's what
	// occupies cells.
	kv := func(name, v string) string {
		l := leet(name)
		gap := "  "
		budget := contentArea - len(l) - len(gap)
		return label(name) + gap + val(trunc(v, budget))
	}
	cwdBudget := contentArea - len(leet("cwd")) - len("   ")
	cwdShort := tail(dash(cwd), cwdBudget)

	rows := []string{
		hdrName.Render(studly("STATION")),
		cDim.Render(strings.Repeat("─", contentArea)),
		kv("mode", modeLabel(mode)),
		kv("node", dash(short(session))),
		kv("agent", dash(modelID)),
		label("cwd") + "   " + val(cwdShort),
		"",
		kv("cost", fmt.Sprintf("$%.4f", cost)),
		kv("turns", fmt.Sprintf("%d", turns)),
	}
	for len(rows) < height {
		rows = append(rows, "")
	}
	if len(rows) > height {
		rows = rows[:height]
	}
	return sidebarBox.Width(sidebarWidth).Height(height).Render(strings.Join(rows, "\n"))
}

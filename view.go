package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) View() string {
	if !m.ready {
		return "starting…"
	}
	if m.splash {
		return splashScreen(m.w, m.h, m.splashFrame, m.logoIdx)
	}
	bg := m.renderBackground()
	// Modal overlays float on top of the live transcript via placeOverlay
	// (ANSI-aware splice — see overlay.go). Only one modal at a time.
	if m.picker != nil {
		return placeOverlay(bg, m.picker.View(), m.w, m.h)
	}
	if m.help {
		return placeOverlay(bg, helpModalView(m.w, m.h), m.w, m.h)
	}
	return bg
}

// renderBackground composes the chrome + transcript + prompt + status. Pulled
// out of View() so modal overlays can splice on top of the same image.
func (m model) renderBackground() string {
	prompt := m.input.View()
	if m.pending != nil {
		prompt = approveBar.Render(fmt.Sprintf(" ►◄ %s  %s    [ENTER] %s (default)    [ESC] %s ◄► ",
			studly("claude wants:"), strings.ToUpper(m.pending.toolName),
			leet("ALLOW"), leet("DENY")))
	}

	bar := bbsScrollbar(m.vp.Height, m.vp.TotalLineCount(), m.vp.VisibleLineCount(), m.vp.YOffset)
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.vp.View(), bar)
	if m.sidebar && m.w >= sidebarMinWidth {
		cwd, _ := os.Getwd()
		turns := 0
		for _, e := range m.entries {
			if e.kind == entUser {
				turns++
			}
		}
		side := bbsSidebar(m.vp.Height, m.mode, m.session, m.modelID, cwd, m.lastCost, turns)
		body = lipgloss.JoinHorizontal(lipgloss.Top, side, body)
	}

	parts := []string{
		bbsBanner(m.w, m.colorPhase, m.headerStyle),
		sceneDivider(leet("session"), m.w),
		body,
	}
	if tray := pendingTray(m.queue, m.w); tray != "" {
		parts = append(parts, tray)
	}
	parts = append(parts,
		prompt,
		bbsStatus(m.mode, m.session, gitBranch(), m.lastCost, m.ctxTokens, m.outTokens, m.ctxLimit, m.busy, m.sp.View(), m.w),
	)
	return strings.Join(parts, "\n")
}

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
// out of View() so modal overlays can splice on top of the same image. The
// transcript body is the memoized frameBody (refreshed in Update); only the
// animated banner and the cheap prompt/status are rebuilt every frame.
func (m model) renderBackground() string {
	// The approval bar replaces the prompt while a decision is pending; otherwise
	// the (possibly multi-line) textarea.
	prompt := m.input.View()
	if m.pending != nil {
		prompt = approveBar.Render(fmt.Sprintf(" ►◄ %s  %s    [ENTER] %s (default)    [ESC] %s ◄► ",
			studly("claude wants:"), strings.ToUpper(m.pending.toolName),
			leet("ALLOW"), leet("DENY")))
	}

	// frameBody is populated by refreshBody in the Update loop; fall back to a
	// fresh compose for callers that render without an Update (tests, asset gen).
	body := m.frameBody
	if body == "" {
		body = m.renderBody()
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
		bbsStatus(m.mode, m.modelID, m.session, gitBranch(), m.lastCost, m.ctxTokens, m.outTokens, m.ctxLimit, m.busy, m.sp.View(), m.w),
	)
	return strings.Join(parts, "\n")
}

// renderBody composes the transcript region: the viewport, its scrollbar, and
// the optional sidebar. This is the expensive part of a frame — re-styling all
// visible lines — so refreshBody memoizes it.
func (m *model) renderBody() string {
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
		side := bbsSidebar(m.vp.Height, m.settings.Sidebar, m.mode, m.session, m.modelID, cwd, m.lastCost, turns)
		if m.settings.Sidebar == sidebarLeft {
			body = lipgloss.JoinHorizontal(lipgloss.Top, side, body)
		} else {
			body = lipgloss.JoinHorizontal(lipgloss.Top, body, side)
		}
	}
	return body
}

// refreshBody recomputes the memoized frameBody only when something it depends
// on changed. Called once per Update; a no-op while typing or animating (the
// viewport, scroll offset, and sidebar data all stay put), which is what keeps
// those frames from re-styling the whole transcript.
func (m *model) refreshBody() {
	k := bodyKey{
		ver: m.contentVer, w: m.vp.Width, h: m.vp.Height, off: m.vp.YOffset, mw: m.w,
		sidebar: m.sidebar, sidePos: m.settings.Sidebar, mode: m.mode, sess: m.session, mid: m.modelID, cost: m.lastCost,
	}
	if k == m.bodyKey && m.frameBody != "" {
		return
	}
	m.bodyKey = k
	m.frameBody = m.renderBody()
}

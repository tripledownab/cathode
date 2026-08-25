// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// gitBranch walks up from cwd looking for a .git/HEAD and returns the current
// branch name (or "@<short-sha>" for detached HEAD, "" if not a repo). Reads
// the file directly — no subprocess — so it's cheap enough to call per render.
func gitBranch() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		b, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
		if err == nil {
			s := strings.TrimSpace(string(b))
			if rest, ok := strings.CutPrefix(s, "ref: refs/heads/"); ok {
				return rest
			}
			if len(s) >= 7 {
				return "@" + s[:7]
			}
			return s
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// abbreviateTokens formats a token count as "12.3K" / "1.2M" so the status
// bar stays compact at high counts.
func abbreviateTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// parseTokenCount accepts "200k", "1m", "500000", etc. Bad input falls back
// to 200K so the gauge still works.
func parseTokenCount(s string) int {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 200_000
	}
	mult := 1
	switch s[len(s)-1] {
	case 'k':
		mult, s = 1_000, s[:len(s)-1]
	case 'm':
		mult, s = 1_000_000, s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return 200_000
	}
	return int(v * float64(mult))
}

// nextCtxTier snaps an exceeded context limit up to the next standard
// Anthropic window. Past 2M we just double — there's no published tier above.
func nextCtxTier(cur int) int {
	switch {
	case cur < 200_000:
		return 200_000
	case cur < 500_000:
		return 500_000
	case cur < 1_000_000:
		return 1_000_000
	case cur < 2_000_000:
		return 2_000_000
	default:
		return cur * 2
	}
}

// shortModel condenses a model id or display label to a status-bar-sized tag:
// the family name when recognizable (opus/sonnet/haiku — covers raw ids like
// "claude-sonnet-4-…" and labels like "Opus (1M context)"), "default" for the
// empty/account-default case, else the first word of whatever claude reported.
func shortModel(s string) string {
	switch l := strings.ToLower(s); {
	case strings.Contains(l, "opus"):
		return "opus"
	case strings.Contains(l, "sonnet"):
		return "sonnet"
	case strings.Contains(l, "haiku"):
		return "haiku"
	case strings.TrimSpace(s) == "":
		return "default"
	default:
		if i := strings.IndexAny(s, " ("); i > 0 {
			return strings.ToLower(strings.TrimSpace(s[:i]))
		}
		return strings.ToLower(s)
	}
}

// bbsStatus renders the DOS-style full-width status line. Each segment is
// styled individually with the cyan background so the bar stays contiguous
// even when nested-style chunks (the context-bar gradient) emit SGR resets.
func bbsStatus(mode, model, session, branch string, cost float64, ctxTok, outTok, ctxLimit int, busy, armed bool, spin string, width int) string {
	if width < 1 {
		width = 1
	}
	state := "READY"
	if busy {
		state = spin + " " + leet("WORKING")
	}
	ctxPct := 0
	if ctxLimit > 0 {
		ctxPct = ctxTok * 100 / ctxLimit
	}

	// Label/value pairs rather than flat strings, so a flat theme can dim the
	// label and keep the value light (statusstyle.go). MDL is the model in use;
	// labelled "MDL" (not "MODEL") so it doesn't leet-render as "M0D3L" right
	// next to MODE's "M0D3".
	plain := [][2]string{
		{leet("MDL"), shortModel(model)},
		{leet("MODE"), modeLabel(mode)},
		{leet("NODE"), short(session)},
	}
	if branch != "" {
		plain = append(plain, [2]string{"BR", branch})
	}

	// CTX is composed inline so the gauge's colored chunks are sandwiched
	// between base-styled wrappers that re-assert the row's background.
	ctxStr := sbarSeg("CTX", abbreviateTokens(ctxTok)) +
		sbarBase.Render(" ") + ctxBar(ctxPct) + sbarBase.Render(" ") +
		sbarValue.Render(fmt.Sprintf("%d%%", ctxPct))
	if ctxPct >= 80 {
		ctxStr = sbarRed.Render("⚠ ") + ctxStr
	}

	sep := sbarBase.Render(" " + ornBullet + " ")
	var b strings.Builder
	b.WriteString(sbarBase.Render(" "))
	for _, p := range plain {
		b.WriteString(sbarSeg(p[0], p[1]))
		b.WriteString(sep)
	}
	b.WriteString(ctxStr)
	b.WriteString(sep + sbarSeg("OUT", abbreviateTokens(outTok)))
	b.WriteString(sep + sbarValue.Render(fmt.Sprintf("$%.4f", cost)))
	left := b.String()

	// The live state indicator — spinner + WORKING while busy, READY otherwise —
	// is pushed flush to the right edge, so the working spinner animates in the
	// bottom-right corner with the gap padding swallowed in the middle. A recent
	// Ctrl+C replaces it with the exit hint until the window lapses.
	//
	// Busy takes its own style: on a flat theme it's the ember, so the one
	// moving thing on screen is also the one colored thing. READY stays a plain
	// value — an idle session should not glow.
	right := sbarValue.Render(state) + sbarBase.Render(" ")
	if busy {
		right = sbarBusy.Render(state) + sbarBase.Render(" ")
	}
	if armed {
		right = sbarRed.Render("^C AGAIN TO EXIT ")
	}
	if gap := width - lipgloss.Width(left) - lipgloss.Width(right); gap > 0 {
		return left + sbarBase.Render(strings.Repeat(" ", gap)) + right
	}
	// Too narrow to right-align without clipping a segment; fall back to an
	// inline separator so nothing is lost.
	return left + sep + right
}

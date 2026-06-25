package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Status-bar palette. Each segment must have its background set explicitly:
// lipgloss inner styles emit a full SGR reset at end-of-chunk, so any nested
// styled run drops cyan — subsequent text would otherwise render on the
// terminal default until the next SGR. Re-asserting bg on every chunk keeps
// the bar visually contiguous.
var (
	sbarBase  lipgloss.Style
	sbarGreen lipgloss.Style
	sbarYel   lipgloss.Style
	sbarRed   lipgloss.Style
)

// buildStatusStyles rebuilds the status-bar styles from the active palette.
// Called from buildStyles (theme.go) on startup and every theme change.
func buildStatusStyles() {
	sbarBase = lipgloss.NewStyle().Bold(true).Foreground(colBlack).Background(colCyan)
	sbarGreen = lipgloss.NewStyle().Bold(true).Foreground(colGreen).Background(colCyan)
	sbarYel = lipgloss.NewStyle().Bold(true).Foreground(colYel).Background(colCyan)
	sbarRed = lipgloss.NewStyle().Bold(true).Foreground(colRed).Background(colCyan)
}

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

// ctxBar is the 8-cell gradient progress bar shown next to the CTX % in the
// status line. Filled blocks shift color as pressure rises: green → yellow
// → red, matching the ⚠ threshold used elsewhere. Empty cells render as ░ in
// the base style so the cyan background carries through cleanly.
func ctxBar(pct int) string {
	const w = 8
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := w * pct / 100
	style := sbarGreen
	switch {
	case pct >= 80:
		style = sbarRed
	case pct >= 60:
		style = sbarYel
	}
	return style.Render(strings.Repeat("█", filled)) + sbarBase.Render(strings.Repeat("░", w-filled))
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
func bbsStatus(mode, model, session, branch string, cost float64, ctxTok, outTok, ctxLimit int, busy bool, spin string, width int) string {
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

	// Plain segments that just need the base style. MDL is the model in use;
	// labelled "MDL" (not "MODEL") so it doesn't leet-render as "M0D3L" right
	// next to MODE's "M0D3".
	plain := []string{
		leet("MDL") + " " + shortModel(model),
		leet("MODE") + " " + modeLabel(mode),
		leet("NODE") + " " + short(session),
	}
	if branch != "" {
		plain = append(plain, "BR "+branch)
	}

	// CTX is composed inline so the bar's colored chunks are sandwiched
	// between two base-styled wrappers that re-assert cyan bg.
	ctxStr := sbarBase.Render(fmt.Sprintf("CTX %s ", abbreviateTokens(ctxTok))) +
		ctxBar(ctxPct) +
		sbarBase.Render(fmt.Sprintf(" %d%%", ctxPct))
	if ctxPct >= 80 {
		ctxStr = sbarRed.Render("⚠ ") + ctxStr
	}

	trailing := []string{
		"OUT " + abbreviateTokens(outTok),
		fmt.Sprintf("$%.4f", cost),
	}

	sep := sbarBase.Render(" " + ornBullet + " ")
	var b strings.Builder
	b.WriteString(sbarBase.Render(" "))
	for _, s := range plain {
		b.WriteString(sbarBase.Render(s))
		b.WriteString(sep)
	}
	b.WriteString(ctxStr)
	for _, s := range trailing {
		b.WriteString(sep)
		b.WriteString(sbarBase.Render(s))
	}
	left := b.String()

	// The live state indicator — spinner + WORKING while busy, READY otherwise —
	// is pushed flush to the right edge, so the working spinner animates in the
	// bottom-right corner with the gap padding swallowed in the middle.
	right := sbarBase.Render(state + " ")
	if gap := width - lipgloss.Width(left) - lipgloss.Width(right); gap > 0 {
		return left + sbarBase.Render(strings.Repeat(" ", gap)) + right
	}
	// Too narrow to right-align without clipping a segment; fall back to an
	// inline separator so nothing is lost.
	return left + sep + right
}

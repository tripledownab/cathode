// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// fuzzyScore returns (score, matched). A higher score is better. Subsequence
// matcher with bonuses for prefix/word-start/consecutive runs and a soft
// penalty for gaps. Returns (0, false) when q isn't a subsequence of s.
func fuzzyScore(q, s string) (int, bool) {
	if q == "" {
		return 1, true
	}
	qs, ss := strings.ToLower(q), strings.ToLower(s)
	qi := 0
	score, streak := 0, 0
	prevWordEnd := true
	for i, c := range ss {
		if qi >= len(qs) {
			break
		}
		// rune-by-rune compare on the lowercased strings
		qc := qs[qi]
		if byte(c) == qc {
			gain := 1
			if i == 0 {
				gain += 8 // prefix bonus
			}
			if prevWordEnd {
				gain += 4 // word-start bonus
			}
			gain += streak * 2 // consecutive bonus
			score += gain
			streak++
			qi++
		} else {
			streak = 0
			score -= 1 // small per-gap penalty
		}
		prevWordEnd = !unicode.IsLetter(c) && !unicode.IsDigit(c)
	}
	if qi < len(qs) {
		return 0, false
	}
	return score, true
}

// pickerItem is one row in the picker. id is what gets returned on selection;
// title and subtitle are what the user sees.
type pickerItem struct {
	id       string
	title    string
	subtitle string
}

// picker is a generic "filter a list, pick one" modal. The `kind` tag tells the
// UI dispatcher what to do with the selection — "sessions" triggers a resume
// re-exec, "slash" runs the chosen command in-process.
type picker struct {
	kind     string
	title    string
	items    []pickerItem
	filtered []int // indices into items, filtered+sorted by the input query
	input    textinput.Model
	cursor   int // index into filtered
	w, h     int

	// twoLine puts the subtitle on its own row under the title, instead of
	// trailing it on the same one. Half as many rows fit, which is the trade:
	// a one-line row truncates the title away first on a narrow terminal, and
	// the title is the part you are reading the list for.
	twoLine bool

	// Checklist mode — see pickermulti.go.
	multi  bool
	marked map[int]bool // marked item indices
}

func newPicker(kind, title string, items []pickerItem, w, h int) *picker {
	ti := textinput.New()
	ti.Placeholder = "filter…"
	ti.Prompt = "› "
	ti.Focus()
	p := &picker{kind: kind, title: title, items: items, input: ti, w: w, h: h}
	p.refilter()
	return p
}

// refilter recomputes p.filtered from the current input. Empty query keeps the
// original order; non-empty runs a fuzzy subsequence scorer (title weighted
// 2× subtitle) and stable-sorts by descending score, with original index as
// the tie-break so the most-recent session still wins ties.
func (p *picker) refilter() {
	q := strings.TrimSpace(p.input.Value())
	p.filtered = p.filtered[:0]
	if q == "" {
		for i := range p.items {
			p.filtered = append(p.filtered, i)
		}
		if p.cursor >= len(p.filtered) {
			p.cursor = 0
		}
		return
	}
	type scored struct{ idx, score int }
	scoredList := make([]scored, 0, len(p.items))
	for i, it := range p.items {
		ts, tok := fuzzyScore(q, it.title)
		ss, sok := fuzzyScore(q, it.subtitle)
		if !tok && !sok {
			continue
		}
		total := ts*2 + ss
		scoredList = append(scoredList, scored{i, total})
	}
	sort.SliceStable(scoredList, func(a, b int) bool {
		return scoredList[a].score > scoredList[b].score
	})
	for _, s := range scoredList {
		p.filtered = append(p.filtered, s.idx)
	}
	if p.cursor >= len(p.filtered) {
		p.cursor = 0
	}
}

// focusedID is the id of the row under the cursor, or "" when nothing matches
// the current filter. Used for live-preview settings (preview the focused value
// before the user commits with Enter).
func (p *picker) focusedID() string {
	if len(p.filtered) == 0 {
		return ""
	}
	return p.items[p.filtered[p.cursor]].id
}

// setCursorTo moves the cursor onto the row with the given id, if present, so a
// picker can open pre-positioned on the current value.
func (p *picker) setCursorTo(id string) {
	for ci, idx := range p.filtered {
		if p.items[idx].id == id {
			p.cursor = ci
			return
		}
	}
}

// Update returns the next picker state and, on selection, the chosen item's id
// (or "" when not yet selected). Returning "" with picker=nil means the user
// pressed Esc to cancel.
func (p *picker) Update(msg tea.Msg) (*picker, string) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return nil, ""
		case " ":
			// Checklist mode spends the space bar on marking, so it never reaches
			// the filter input. Single-select pickers still type it.
			if p.multi {
				p.toggleMarked()
				return p, ""
			}
		case "enter":
			if p.multi && len(p.marked) > 0 {
				return nil, p.markedID()
			}
			if len(p.filtered) == 0 {
				return p, ""
			}
			return nil, p.items[p.filtered[p.cursor]].id
		case "up", "ctrl+p":
			if p.cursor > 0 {
				p.cursor--
			}
			return p, ""
		case "down", "ctrl+n":
			if p.cursor < len(p.filtered)-1 {
				p.cursor++
			}
			return p, ""
		}
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	_ = cmd
	p.refilter()
	return p, ""
}

// View renders the picker as a CP437-bordered dialog. Width is clamped so it
// looks reasonable even in tiny terminals.
func (p *picker) View() string {
	w := p.w - 4
	if w < 40 {
		w = 40
	}
	if w > 100 {
		w = 100
	}
	maxRows := p.h - 8
	if maxRows < 4 {
		maxRows = 4
	}
	if maxRows > 16 {
		maxRows = 16
	}
	// maxRows is screen rows; the window is measured in ITEMS. A two-line item
	// takes two rows, so half as many fit — without this the list renders past
	// the bottom of the box and the scrollbar no longer matches the text.
	maxItems := maxRows
	if p.twoLine {
		maxItems = maxRows / 2
		if maxItems < 2 {
			maxItems = 2
		}
	}

	innerW := w - 2     // text area inside the box's 1-col padding
	listW := innerW - 1 // reserve the last column for the scrollbar gutter

	header := dTitle.Render(" "+p.title+" ") + "\n" + p.input.View()
	footer := cDim.Render("  [↑↓] move   [enter] choose   [esc] cancel")
	if p.multi {
		footer = cDim.Render("  [↑↓] move   [space] mark   [enter] confirm   [esc] cancel")
	}

	section := cDim.Render("  (no matches)")
	if len(p.filtered) > 0 {
		start := 0
		if p.cursor >= maxItems {
			start = p.cursor - maxItems + 1
		}
		end := start + maxItems
		if end > len(p.filtered) {
			end = len(p.filtered)
		}
		// ANSI-aware truncate + space-pad to a fixed width so the scrollbar
		// lands as a straight column regardless of styled content.
		pad := func(line string, w int) string {
			line = ansi.Truncate(line, w, "")
			if n := w - lipgloss.Width(line); n > 0 {
				line += strings.Repeat(" ", n)
			}
			return line
		}
		fit := func(line string) string { return pad(line, listW) }
		rows := make([]string, 0, (end-start)*2)
		for i := start; i < end; i++ {
			idx := p.filtered[i]
			it := p.items[idx]
			mk := p.mark(idx)
			sel := i == p.cursor

			if !p.twoLine {
				line := fmt.Sprintf("  %s%s   %s", mk, it.title, cDim.Render(it.subtitle))
				if sel {
					line = approveBar.Render(" " + mk + it.title + "   " + it.subtitle + " ")
				}
				rows = append(rows, fit(line))
				continue
			}

			// Two rows per item. A selected one is padded to the same width
			// BEFORE styling, so both lines highlight to the same length — style
			// first and the lightbar is two ragged blocks, because a title and
			// its detail line are never the same length.
			head, sub := "  "+mk+it.title, "    "+it.subtitle
			if sel {
				// approveBar carries Padding(0, 1), so the content is padded to
				// two columns short and the styled row lands at exactly listW.
				// Pad to the full width and the row wraps, which desyncs the
				// scrollbar gutter from the text beside it.
				inner := listW - 2
				if inner < 1 {
					inner = 1
				}
				rows = append(rows, approveBar.Render(pad(head, inner)))
				rows = append(rows, approveBar.Render(pad(sub, inner)))
				continue
			}
			rows = append(rows, fit(head), fit(cDim.Render(sub)))
		}
		// Themed BBS scrollbar gutter, mirroring the transcript's — replaces the
		// old "N more…" text so a long session/command list scrolls in-theme.
		// Measured in items, not rows: the thumb tracks position in the list,
		// and its height is the gutter's, which is however many rows we drew.
		bar := bbsScrollbar(len(rows), len(p.filtered), end-start, start, true)
		section = lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(rows, "\n"), bar)
	}

	body := header + "\n" + section + "\n" + footer
	box := lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(colCyan).Padding(0, 1).Width(w)
	return box.Render(body)
}

package main

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
		case "enter":
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

	var b strings.Builder
	b.WriteString(dTitle.Render(" " + p.title + " "))
	b.WriteString("\n")
	b.WriteString(p.input.View())
	b.WriteString("\n")

	if len(p.filtered) == 0 {
		b.WriteString(cDim.Render("  (no matches)"))
	} else {
		start := 0
		if p.cursor >= maxRows {
			start = p.cursor - maxRows + 1
		}
		end := start + maxRows
		if end > len(p.filtered) {
			end = len(p.filtered)
		}
		for i := start; i < end; i++ {
			it := p.items[p.filtered[i]]
			line := fmt.Sprintf("  %s   %s", it.title, cDim.Render(it.subtitle))
			if i == p.cursor {
				line = approveBar.Render(" " + it.title + "   " + it.subtitle + " ")
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
		if len(p.filtered) > maxRows {
			b.WriteString(cDim.Render(fmt.Sprintf("  %d more…", len(p.filtered)-maxRows)))
			b.WriteString("\n")
		}
	}
	b.WriteString(cDim.Render("  [↑↓] move   [enter] choose   [esc] cancel"))

	box := lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(colCyan).Padding(0, 1).Width(w)
	return box.Render(strings.TrimRight(b.String(), "\n"))
}

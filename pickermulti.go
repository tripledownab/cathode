// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

// Checklist mode for the picker.
//
// Claude's AskUserQuestion marks a question `multiSelect` when several answers
// are valid at once (question.go). Such a picker marks rows with space and
// answers with every marked row on Enter. Enter with nothing marked still
// answers with the row under the cursor, so a one-keypress answer keeps
// working and a single-select picker is untouched.

// setMulti turns on checklist mode.
func (p *picker) setMulti() {
	p.multi = true
	p.marked = map[int]bool{}
}

// toggleMarked marks or unmarks the row under the cursor. It does nothing when
// the filter matches no row.
func (p *picker) toggleMarked() {
	if len(p.filtered) == 0 {
		return
	}
	idx := p.filtered[p.cursor]
	if p.marked[idx] {
		delete(p.marked, idx)
		return
	}
	p.marked[idx] = true
}

// markedID is the first marked item's id in item order, or "" when no row is
// marked. Update reports it as the selection so Enter still closes the picker
// when the live filter hides every marked row.
func (p *picker) markedID() string {
	for i := range p.items {
		if p.marked[i] {
			return p.items[i].id
		}
	}
	return ""
}

// chosenIDs expands the id that Update returned into the full answer: every
// marked row in item order, or the single chosen row.
func (p *picker) chosenIDs(chosen string) []string {
	if !p.multi || len(p.marked) == 0 {
		return []string{chosen}
	}
	ids := make([]string, 0, len(p.marked))
	for i := range p.items {
		if p.marked[i] {
			ids = append(ids, p.items[i].id)
		}
	}
	return ids
}

// mark is the checkbox drawn at the head of a row. It is empty outside
// checklist mode, which keeps every other picker's layout as it was.
func (p *picker) mark(idx int) string {
	switch {
	case !p.multi:
		return ""
	case p.marked[idx]:
		return "[x] "
	default:
		return "[ ] "
	}
}

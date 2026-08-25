// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func askReq(input string) approvalReq {
	return approvalReq{
		toolName: askUserQuestionTool,
		input:    json.RawMessage(input),
		reply:    make(chan approvalReply, 1),
	}
}

// An AskUserQuestion approval opens a question picker (never the y/n bar), and
// answering it replies via deny+message with clear Q→A pairs.
func TestAskQuestionSingle(t *testing.T) {
	req := askReq(`{"questions":[{"question":"Tabs or spaces?","header":"Indent","options":[{"label":"Tabs","description":"t"},{"label":"Spaces","description":"s"}]}]}`)
	m := model{w: 80, h: 24, approvals: &Approvals{}}

	next, _ := m.Update(pendingApprovalMsg{req: req})
	nm := next.(model)
	if nm.pending != nil {
		t.Fatal("AskUserQuestion must not raise the y/n approval bar")
	}
	if nm.question == nil || nm.picker == nil || nm.picker.kind != "question" {
		t.Fatalf("expected a question picker (question=%v picker=%v)", nm.question != nil, nm.picker != nil)
	}

	if cmd := nm.answerQuestion([]string{"Spaces"}); cmd == nil {
		t.Fatal("answering the only question should reply and re-arm the waiter")
	}
	d := <-req.reply
	if d.allow {
		t.Fatal("a question answer is delivered via deny+message, not allow")
	}
	if !strings.Contains(d.message, "Spaces") || !strings.Contains(d.message, "Tabs or spaces?") {
		t.Fatalf("answer message should carry the Q and A, got %q", d.message)
	}
	if nm.question != nil {
		t.Error("question state should clear once answered")
	}
}

// Multiple questions are asked in sequence; the reply lands only after the last.
func TestAskQuestionSequence(t *testing.T) {
	req := askReq(`{"questions":[
		{"question":"Q1?","options":[{"label":"A"},{"label":"B"}]},
		{"question":"Q2?","options":[{"label":"C"},{"label":"D"}]}]}`)
	q, ok := parseAskQuestion(req)
	if !ok || len(q.questions) != 2 {
		t.Fatalf("parse failed ok=%v", ok)
	}
	m := model{w: 80, h: 24, approvals: &Approvals{}, question: q}
	m.picker = q.picker(m.w, m.h)

	if cmd := m.answerQuestion([]string{"A"}); cmd != nil {
		t.Fatal("first of two answers should not reply yet")
	}
	if m.question.idx != 1 || m.picker == nil || m.picker.kind != "question" {
		t.Fatal("second question should open next")
	}
	select {
	case <-req.reply:
		t.Fatal("must not reply before all questions answered")
	default:
	}
	if cmd := m.answerQuestion([]string{"D"}); cmd == nil {
		t.Fatal("final answer should reply")
	}
	d := <-req.reply
	if !strings.Contains(d.message, "A") || !strings.Contains(d.message, "D") {
		t.Fatalf("combined answer should include both choices: %q", d.message)
	}
}

// A multiSelect question marks rows with space and answers with all of them on
// Enter, driven through the real key path.
func TestAskQuestionMultiSelect(t *testing.T) {
	req := askReq(`{"questions":[{"question":"Which features?","multiSelect":true,"options":[
		{"label":"Auth"},{"label":"Billing"},{"label":"Search"}]}]}`)
	m := newModel(&Engine{}, "ask", &Approvals{}, "bar", "")
	m.splash = false // the splash would eat the first keypress

	next, _ := m.Update(pendingApprovalMsg{req: req})
	nm := next.(model)
	if nm.picker == nil || !nm.picker.multi {
		t.Fatal("a multiSelect question should open a checklist picker")
	}

	space := tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	for _, key := range []tea.KeyMsg{
		space,                // mark Auth
		{Type: tea.KeyDown},  // → Billing
		{Type: tea.KeyDown},  // → Search
		space,                // mark Search
		{Type: tea.KeyEnter}, // confirm both
	} {
		n, _ := nm.Update(key)
		nm = n.(model)
	}

	d := <-req.reply
	if !strings.Contains(d.message, "Auth, Search") {
		t.Fatalf("both marked options should reach claude, got %q", d.message)
	}
	if strings.Contains(d.message, "Billing") {
		t.Fatalf("an unmarked option must not be answered, got %q", d.message)
	}
	if nm.question != nil || nm.picker != nil {
		t.Error("the question should close once confirmed")
	}
}

// Marking nothing keeps the one-keypress answer: Enter takes the focused row.
func TestAskQuestionMultiSelectEnterTakesFocused(t *testing.T) {
	req := askReq(`{"questions":[{"question":"Which?","multiSelect":true,"options":[
		{"label":"One"},{"label":"Two"}]}]}`)
	m := newModel(&Engine{}, "ask", &Approvals{}, "bar", "")
	m.splash = false

	next, _ := m.Update(pendingApprovalMsg{req: req})
	nm := next.(model)
	for _, key := range []tea.KeyMsg{{Type: tea.KeyDown}, {Type: tea.KeyEnter}} {
		n, _ := nm.Update(key)
		nm = n.(model)
	}

	d := <-req.reply
	if !strings.Contains(d.message, "Two") || strings.Contains(d.message, "One") {
		t.Fatalf("Enter alone should answer with the focused row, got %q", d.message)
	}
}

// A non-question tool falls back to the normal allow/deny flow.
func TestParseAskQuestionRejectsOthers(t *testing.T) {
	if _, ok := parseAskQuestion(approvalReq{toolName: "Edit", input: json.RawMessage(`{}`)}); ok {
		t.Error("Edit is not an AskUserQuestion")
	}
	if _, ok := parseAskQuestion(askReq(`{"questions":[]}`)); ok {
		t.Error("empty questions should not parse")
	}
}

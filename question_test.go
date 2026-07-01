package main

import (
	"encoding/json"
	"strings"
	"testing"
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

	if cmd := nm.answerQuestion("Spaces"); cmd == nil {
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

	if cmd := m.answerQuestion("A"); cmd != nil {
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
	if cmd := m.answerQuestion("D"); cmd == nil {
		t.Fatal("final answer should reply")
	}
	d := <-req.reply
	if !strings.Contains(d.message, "A") || !strings.Contains(d.message, "D") {
		t.Fatalf("combined answer should include both choices: %q", d.message)
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

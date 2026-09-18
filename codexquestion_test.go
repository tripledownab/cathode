// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// The request shape is taken from ToolRequestUserInputParams in the codex
// app-server schema (0.153.4), not guessed.
func TestCodexQuestionBecomesTheSharedPicker(t *testing.T) {
	in := codexQuestionParams{ItemID: "item-7", Questions: []codexQuestion{{
		ID:       "q1",
		Header:   "Backend",
		Question: "Which store should this write to?",
		Options: []codexQuestionOption{
			{Label: "sqlite", Description: "a file on disk"},
			{Label: "postgres", Description: "the shared server"},
		},
	}}}

	raw, ok := codexAskInput(in)
	if !ok {
		t.Fatal("a question with options must be askable")
	}

	q, ok := parseAskQuestion(approvalReq{toolName: askUserQuestionTool, input: raw})
	if !ok {
		t.Fatal("the translated input must parse as an AskUserQuestion")
	}
	cur := q.current()
	if cur.ID != "q1" {
		t.Errorf("id = %q, want the id codex files the answer under", cur.ID)
	}
	if cur.Question != in.Questions[0].Question || cur.Header != in.Questions[0].Header {
		t.Errorf("question = %q / %q, want it carried through", cur.Question, cur.Header)
	}
	if len(cur.Options) != 2 || cur.Options[1].Description != "the shared server" {
		t.Errorf("options = %+v, want both with their descriptions", cur.Options)
	}
}

// A question the picker cannot put to the user is dropped rather than shown
// with no way to answer it.
func TestCodexQuestionSkipsWhatThePickerCannotAsk(t *testing.T) {
	opts := []codexQuestionOption{{Label: "yes", Description: ""}}
	for _, c := range []struct {
		name string
		q    codexQuestion
	}{
		{"free text", codexQuestion{ID: "q", Question: "Name it?"}},
		{"secret", codexQuestion{ID: "q", Question: "Token?", Options: opts, IsSecret: true}},
		{"no id", codexQuestion{Question: "Which?", Options: opts}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if codexAskable(c.q) {
				t.Fatalf("%+v must not be askable", c.q)
			}
			if _, ok := codexAskInput(codexQuestionParams{Questions: []codexQuestion{c.q}}); ok {
				t.Error("a request of only unaskable questions must report none")
			}
		})
	}

	// Mixed: the askable one is still asked, and the rest are left unanswered.
	raw, ok := codexAskInput(codexQuestionParams{Questions: []codexQuestion{
		{ID: "free", Question: "Name it?"},
		{ID: "pick", Question: "Which?", Options: opts},
	}})
	if !ok {
		t.Fatal("one askable question is enough to ask")
	}
	var parsed askInput
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Questions) != 1 || parsed.Questions[0].ID != "pick" {
		t.Errorf("asked %+v, want only the one with options", parsed.Questions)
	}
}

func TestCodexQuestionResultKeysAnswersByID(t *testing.T) {
	got := codexQuestionResult([]questionAnswer{
		{id: "q1", labels: []string{"sqlite"}},
		{id: "q2", labels: []string{"read", "write"}},
		{id: "q3"}, // dismissed: absent, not empty
	})
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"q1":{"answers":["sqlite"]}`, `"q2":{"answers":["read","write"]}`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("result %s, want it to contain %s", b, want)
		}
	}
	if strings.Contains(string(b), `"q3"`) {
		t.Errorf("result %s, want an unanswered id left out entirely", b)
	}
}

// End to end over the wire: codex asks, the pane answers, and the reply carries
// the chosen label under the question's id. An unanswered request stops the turn
// dead, so the reply reaching the wire is the point of the test.
func TestCodexQuestionRoundTrip(t *testing.T) {
	fakeAppServer(t, `
while IFS= read -r line; do
  case "$line" in
    *'"initialize"'*) printf '{"method":"item/tool/requestUserInput","id":0,"params":{"itemId":"item-7","threadId":"th-1","turnId":"tu-1","isBlocking":true,"questions":[{"id":"q1","header":"Store","question":"Which store?","options":[{"label":"sqlite","description":"a file"},{"label":"postgres","description":"the server"}]}]}}\n' ;;
    *'"answers"'*)    printf '{"method":"cathode/test/echo","params":%s}\n' "$line" ;;
  esac
done
`)
	e, err := newCodexEngine(codexEngineConfig{Mode: "build"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	frames := make(chan codexFrame, 16)
	other := make(chan tea.Msg, 16)
	sinkTo(e, frames, other)
	go func() { _, _ = e.call("initialize", map[string]any{}) }()

	req := awaitApproval(t, other)
	if req.toolName != askUserQuestionTool {
		t.Fatalf("toolName = %q, want the shared question tool so the picker opens", req.toolName)
	}
	if req.toolUseID != "item-7" {
		t.Errorf("toolUseID = %q, want the itemId", req.toolUseID)
	}
	req.reply <- approvalReply{answers: []questionAnswer{{id: "q1", labels: []string{"postgres"}}}}

	select {
	case f := <-frames:
		if !strings.Contains(string(f.Params), `"q1":{"answers":["postgres"]}`) {
			t.Errorf("sent %s, want the answer under its question id", f.Params)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no answer was sent; the turn would hang")
	}
}

// Answering must send both forms: the prose claude reads and the ids codex
// reads. They are filled at one call site, so dropping either one leaves the
// other backend's answer empty with nothing else failing.
func TestAnsweringSendsBothFormsOfTheAnswer(t *testing.T) {
	raw, ok := codexAskInput(codexQuestionParams{Questions: []codexQuestion{
		{ID: "q1", Question: "Which store?", Options: []codexQuestionOption{{Label: "sqlite"}, {Label: "postgres"}}},
		{ID: "q2", Question: "Which mode?", Options: []codexQuestionOption{{Label: "read"}}},
	}})
	if !ok {
		t.Fatal("both questions offer options")
	}
	reply := make(chan approvalReply, 1)
	q, ok := parseAskQuestion(approvalReq{toolName: askUserQuestionTool, input: raw, reply: reply})
	if !ok {
		t.Fatal("the translated input must parse")
	}

	m, _ := newTestModel(t, "")
	m.question = q
	m.answerQuestion([]string{"postgres"})
	if m.question == nil {
		t.Fatal("two questions were asked; the first answer must not finish them")
	}
	m.answerQuestion([]string{"read"})

	got := <-reply
	if !strings.Contains(got.message, "postgres") || !strings.Contains(got.message, "read") {
		t.Errorf("message = %q, want both answers in the prose claude reads", got.message)
	}
	b, err := json.Marshal(codexQuestionResult(got.answers))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"q1":{"answers":["postgres"]}`, `"q2":{"answers":["read"]}`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("codex reply %s, want it to contain %s", b, want)
		}
	}
}

// A question is asked even in build mode. It is not a permission, so "go
// autonomously" is not an answer to it.
func TestCodexQuestionIsAskedEvenInBuildMode(t *testing.T) {
	raw, ok := codexAskInput(codexQuestionParams{ItemID: "i", Questions: []codexQuestion{{
		ID: "q1", Question: "Which?", Options: []codexQuestionOption{{Label: "a"}, {Label: "b"}},
	}}})
	if !ok {
		t.Fatal(raw)
	}
	m, _ := newTestModel(t, "")
	m.mode = "build"
	reply := make(chan approvalReply, 1)
	out, _ := m.Update(pendingApprovalMsg{req: approvalReq{
		toolName: askUserQuestionTool, toolUseID: "i", input: raw, reply: reply,
	}})
	got := out.(model)
	if got.question == nil || got.picker == nil {
		t.Fatal("build mode must still open the question picker")
	}
	select {
	case r := <-reply:
		t.Fatalf("build mode auto-answered the question with %+v", r)
	default:
	}
}

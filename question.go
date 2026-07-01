package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AskUserQuestion support.
//
// When Claude asks a question it routes through our approval tool as
// toolName "AskUserQuestion" with the questions/options in the input. The
// headless CLI can't answer it interactively (it would just error), so we
// intercept it here: present each question's options as a picker and feed the
// chosen labels back through the permission reply's message (see Approvals).
// Claude reads that message as the answer and continues.

const askUserQuestionTool = "AskUserQuestion"

type askOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type askQuestion struct {
	Question    string      `json:"question"`
	Header      string      `json:"header"`
	MultiSelect bool        `json:"multiSelect"`
	Options     []askOption `json:"options"`
}

type askInput struct {
	Questions []askQuestion `json:"questions"`
}

// pendingQuestion is the in-flight AskUserQuestion: the request to reply on, the
// parsed questions, and the answers collected so far (one per question, asked in
// order).
type pendingQuestion struct {
	req       approvalReq
	questions []askQuestion
	answers   []string
	idx       int
}

// parseAskQuestion returns the pending question for an AskUserQuestion approval
// request, or (nil,false) for anything else (which falls back to the normal
// allow/deny approval). Requires at least one question, each with options.
func parseAskQuestion(req approvalReq) (*pendingQuestion, bool) {
	if req.toolName != askUserQuestionTool {
		return nil, false
	}
	var in askInput
	if json.Unmarshal(req.input, &in) != nil || len(in.Questions) == 0 {
		return nil, false
	}
	for _, q := range in.Questions {
		if len(q.Options) == 0 {
			return nil, false
		}
	}
	return &pendingQuestion{req: req, questions: in.Questions}, true
}

// current is the question awaiting an answer.
func (q *pendingQuestion) current() askQuestion { return q.questions[q.idx] }

// picker builds the options picker for the current question.
func (q *pendingQuestion) picker(w, h int) *picker {
	cur := q.current()
	items := make([]pickerItem, 0, len(cur.Options))
	for _, o := range cur.Options {
		items = append(items, pickerItem{id: o.Label, title: o.Label, subtitle: o.Description})
	}
	title := cur.Question
	if n := len(q.questions); n > 1 {
		title = fmt.Sprintf("(%d/%d) %s", q.idx+1, n, cur.Question)
	}
	return newPicker("question", title, items, w, h)
}

// answerMessage is the text Claude receives (via the denial message) once every
// question is answered — clear "question: answer" pairs it can act on.
func (q *pendingQuestion) answerMessage() string {
	var b strings.Builder
	b.WriteString("The user answered:")
	for i, a := range q.answers {
		b.WriteString(fmt.Sprintf("\n- %s → %s", q.questions[i].Question, a))
	}
	b.WriteString("\n\nUse these answers and continue.")
	return b.String()
}

// summary is the compact transcript note recorded once answered.
func (q *pendingQuestion) summary() string {
	return "answered: " + strings.Join(q.answers, " · ")
}

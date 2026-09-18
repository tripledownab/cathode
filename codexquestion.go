// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import "encoding/json"

// ---- codex asks the user a question ----
//
// item/tool/requestUserInput is codex's analogue of claude's AskUserQuestion,
// and it carries the same thing: a list of questions, each offering labelled
// options. So this translates the request into the shape question.go already
// parses, and the chosen labels back into the result codex expects. The picker,
// the one-question-at-a-time flow and the transcript note are all shared.
//
// Two differences shape the code below. codex keys each answer by a question
// id rather than by position, so the id rides out with the question and back
// with the answer. And a codex question may offer no options at all, or be
// marked secret; the picker can do neither, so those questions are left out of
// the answer map. codex reads a missing id as unanswered, which is exactly what
// happened.
//
// Two schema fields are narrowed away on purpose. isOther offers a free-form
// answer beside the options, which the picker has no way to take — the same
// narrowing it already applies to claude's questions. And isBlocking is unread:
// answering is correct either way, and it is not answering that stops a turn.

const codexQuestionMethod = "item/tool/requestUserInput"

type codexQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type codexQuestion struct {
	ID       string                `json:"id"`
	Header   string                `json:"header"`
	Question string                `json:"question"`
	Options  []codexQuestionOption `json:"options"`
	IsSecret bool                  `json:"isSecret"`
}

type codexQuestionParams struct {
	ItemID    string          `json:"itemId"`
	Questions []codexQuestion `json:"questions"`
}

// codexAskable reports whether cathode can put this question to the user. It
// needs an id to file the answer under, and at least one option to choose from.
// A secret answer is refused whatever it offers, because the picker prints the
// choice into the transcript.
func codexAskable(q codexQuestion) bool {
	return q.ID != "" && len(q.Options) > 0 && !q.IsSecret
}

// codexAskInput translates the askable questions into the AskUserQuestion input
// that question.go parses. It reports false when none of them are askable,
// which tells the caller to answer with nothing rather than open an empty
// picker.
func codexAskInput(p codexQuestionParams) (json.RawMessage, bool) {
	var in askInput
	for _, q := range p.Questions {
		if !codexAskable(q) {
			continue
		}
		opts := make([]askOption, 0, len(q.Options))
		for _, o := range q.Options {
			opts = append(opts, askOption{Label: o.Label, Description: o.Description})
		}
		in.Questions = append(in.Questions, askQuestion{
			ID: q.ID, Question: q.Question, Header: q.Header, Options: opts,
		})
	}
	if len(in.Questions) == 0 {
		return nil, false
	}
	b, err := json.Marshal(in)
	if err != nil {
		return nil, false
	}
	return b, true
}

// codexQuestionResult builds the reply: each answered question id mapped to the
// labels chosen for it. An unanswered id is absent rather than empty.
func codexQuestionResult(answers []questionAnswer) map[string]any {
	out := map[string]any{}
	for _, a := range answers {
		if a.id == "" || len(a.labels) == 0 {
			continue
		}
		out[a.id] = map[string]any{"answers": a.labels}
	}
	return map[string]any{"answers": out}
}

// askQuestion puts codex's question to the user and answers with what they
// chose. It goes to the pane through ask, like an approval does.
func (e *codexEngine) askQuestion(id int64, f codexFrame) {
	var p codexQuestionParams
	_ = json.Unmarshal(f.Params, &p)

	input, ok := codexAskInput(p)
	if !ok {
		// Nothing here cathode can ask. Answer with no answers, which is the
		// dismissed-question case, and say why — a question that never appears
		// reads as a bug in the pane rather than a limit of it.
		e.replyResult(id, codexQuestionResult(nil))
		e.emitError("unanswered question: cathode can only answer one that offers options and is not secret")
		return
	}

	e.ask(id, approvalReq{
		toolName:  askUserQuestionTool,
		toolUseID: p.ItemID,
		input:     input,
	}, func(r approvalReply) any { return codexQuestionResult(r.answers) })
}

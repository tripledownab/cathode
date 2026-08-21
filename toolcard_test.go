package main

import (
	"encoding/json"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
)

// toolCardModel is a model ready to take both halves of one tool call: the
// assistant stream event and the approval request.
func toolCardModel() *model {
	m := newModel(&Engine{}, "ask", nil, "bar", "")
	m.vp = viewport.New(80, 24)
	m.ready = true
	return &m
}

const toolCardInput = `{"file_path":"/tmp/a.txt","old_string":"a","new_string":"b"}`

func editEvent(id string) Envelope {
	return Envelope{Type: "assistant", Message: &APIMessage{
		Role: "assistant",
		Content: []ContentBlock{{
			Type: "tool_use", ID: id, Name: "Edit",
			Input: json.RawMessage(toolCardInput),
		}},
	}}
}

// approve feeds one approval request through Update. Update takes the model by
// value, so the mutated model is the one it returns — the tests thread it back.
func approve(m *model, id string) *model {
	next, _ := m.Update(editApproval(id))
	updated := next.(model)
	return &updated
}

func editApproval(id string) pendingApprovalMsg {
	return pendingApprovalMsg{req: approvalReq{
		toolName:  "Edit",
		toolUseID: id,
		input:     json.RawMessage(toolCardInput),
		reply:     make(chan approvalReply, 1),
	}}
}

func countDiffs(m *model) int {
	n := 0
	for _, e := range m.entries {
		if e.kind == entDiff {
			n++
		}
	}
	return n
}

// TestToolCardDrawnOnce pins the duplicate-diff fix. In ask mode the assistant
// event and the approval request both describe one tool call, and both used to
// append a diff — so every edit was rendered twice. Either order must leave
// exactly one card.
func TestToolCardDrawnOnce(t *testing.T) {
	t.Run("stream first", func(t *testing.T) {
		m := toolCardModel()
		m.handleEvent(editEvent("toolu_1"))
		m = approve(m, "toolu_1")
		if got := countDiffs(m); got != 1 {
			t.Fatalf("stream then approval: got %d diff entries, want 1", got)
		}
	})

	t.Run("approval first", func(t *testing.T) {
		m := toolCardModel()
		m = approve(m, "toolu_1")
		m.handleEvent(editEvent("toolu_1"))
		if got := countDiffs(m); got != 1 {
			t.Fatalf("approval then stream: got %d diff entries, want 1", got)
		}
	})
}

// TestToolCardDistinctCallsBothDrawn guards the other direction: two real edits
// of the same file with the same input are two cards, not one.
func TestToolCardDistinctCallsBothDrawn(t *testing.T) {
	m := toolCardModel()
	m.handleEvent(editEvent("toolu_1"))
	m.handleEvent(editEvent("toolu_2"))
	if got := countDiffs(m); got != 2 {
		t.Fatalf("two tool_use ids: got %d diff entries, want 2", got)
	}
}

// TestToolCardWithoutIDStillDrawn documents the fallback: a client that omits
// tool_use_id from the approve call has nothing to pair on, so the approval
// path keeps drawing its own card rather than dropping the record.
func TestToolCardWithoutIDStillDrawn(t *testing.T) {
	m := toolCardModel()
	m.handleEvent(editEvent("toolu_1"))
	m = approve(m, "")
	if got := countDiffs(m); got != 2 {
		t.Fatalf("approval without an id: got %d diff entries, want 2", got)
	}
}

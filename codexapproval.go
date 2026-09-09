// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
)

// ---- answering the requests codex makes of us ----
//
// The app-server sends requests in the other direction, and every one of them
// blocks until answered. An unanswered request is not a dropped message: the
// turn stops there and the session looks frozen with no error anywhere. So the
// rule here is that every server request gets a reply, always — either the
// user's decision, or a refusal, but never silence.
//
// A gated action goes to the same pane claude's approvals use. That reuse is
// the point: the y/n bar, the diff card, the AskUserQuestion picker and the
// build-mode short-circuit are all shared, and the only codex-specific part is
// turning a decision back into a JSON-RPC response.

// codexApprovalMethods are the server requests that gate an action, and so can
// be answered with a decision. Anything not on this list is answered with a
// JSON-RPC error instead — inventing a reply for a request whose semantics we
// have not established is worse than declining it plainly.
var codexApprovalMethods = map[string]bool{
	"item/commandExecution/requestApproval": true,
	"item/fileChange/requestApproval":       true,
	"item/permissions/requestApproval":      true,
	"applyPatchApproval":                    true,
	"execCommandApproval":                   true,
}

// codexRefusal is the decision sent when the user declines, and when a request
// has to be refused without asking.
//
// "cancel" and not "decline", and deliberately not read from the request's
// availableDecisions: a file-change approval offers only ["accept"], so there is
// no refusal in the offered set at all. Probing the live app-server showed
// cancel is accepted anyway and ends the turn cleanly with TurnAborted, rather
// than being rejected as an unknown variant. That makes it the one refusal that
// works for every request shape.
const (
	codexRefusal  = "cancel"
	codexApproval = "accept"
)

// codexApprovalParams is the part of a gating request this needs. The command
// approval carries the command; the file-change approval carries ids and
// nothing else, which is why the card is drawn from the earlier item/started
// and paired by itemId (toolcard.go).
type codexApprovalParams struct {
	ItemID  string `json:"itemId"`
	Command string `json:"command"`
}

// answerServerRequest replies to a request from the app-server. It never
// declines to answer: see the file comment for what silence costs.
func (e *codexEngine) answerServerRequest(f codexFrame) {
	if f.ID == nil {
		return
	}
	if !codexApprovalMethods[f.Method] {
		// Not an approval. Answer with the JSON-RPC "method not found" code,
		// which is a well-defined way to say "this client cannot do that" and
		// leaves the server to decide what happens next.
		e.replyError(*f.ID, -32601, "cathode does not implement "+f.Method)
		e.emitError("unhandled request " + f.Method)
		return
	}
	e.askUser(*f.ID, f)
}

// askUser puts a gated action in front of the user and answers with what they
// choose.
//
// All of it runs off the reader goroutine, for two separate reasons. The reader
// must keep draining while the pane is up, or the item events that draw the
// very card being approved never arrive. And approvals are admitted one at a
// time: the UI holds a single pending approval, so a second would overwrite the
// first and leave codex waiting on a reply that can no longer be given.
func (e *codexEngine) askUser(id int64, f codexFrame) {
	var p codexApprovalParams
	_ = json.Unmarshal(f.Params, &p)

	go func() {
		e.approvalSlot <- struct{}{}
		defer func() { <-e.approvalSlot }()

		reply := make(chan approvalReply, 1)
		e.emitMsg(pendingApprovalMsg{req: approvalReq{
			toolName:  codexApprovalLabel(f.Method, p),
			toolUseID: p.ItemID,
			input:     f.Params,
			reply:     reply,
		}})

		decision := codexRefusal
		if (<-reply).allow {
			decision = codexApproval
		}
		e.replyResult(id, map[string]any{"decision": decision})
	}()
}

// codexApprovalLabel names the action on the approval bar. The command itself
// is far more use than the method name, so prefer it where the request carries
// one; the rest fall back to the item kind.
func codexApprovalLabel(method string, p codexApprovalParams) string {
	if c := strings.TrimSpace(p.Command); c != "" {
		return c
	}
	switch method {
	case "item/fileChange/requestApproval":
		return "file change"
	case "item/permissions/requestApproval":
		return "permission"
	}
	return method
}

// replyResult and replyError are the two shapes of answer, kept apart so a
// caller cannot half-fill one.
func (e *codexEngine) replyResult(id int64, result any) {
	_ = e.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (e *codexEngine) replyError(id int64, code int, msg string) {
	_ = e.write(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": msg},
	})
}

// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

// ---- answering the requests codex makes of us ----
//
// The app-server sends requests in the other direction, and every one of them
// blocks until answered. An unanswered request is not a dropped message: the
// turn stops there and the session looks frozen with no error anywhere. So this
// file's rule is that every server request gets a reply, always.
//
// The approval pane is not wired to codex yet. Until it is, a gated action is
// refused rather than granted, because the alternative is a backend that
// silently runs whatever it likes in the mode whose entire purpose is asking
// first.

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

// codexRefusal is the decision sent for a gated action.
//
// "cancel" and not "decline", and deliberately not read from the request's
// availableDecisions: a file-change approval offers only ["accept"], so there is
// no refusal in the offered set at all. Probing the live app-server showed
// cancel is accepted anyway and ends the turn cleanly with TurnAborted, rather
// than being rejected as an unknown variant. That makes it the one refusal that
// works for every request shape.
const codexRefusal = "cancel"

// answerServerRequest replies to a request from the app-server. It never
// declines to answer: see the file comment for what silence costs.
func (e *codexEngine) answerServerRequest(f codexFrame) {
	if f.ID == nil {
		return
	}
	if codexApprovalMethods[f.Method] {
		_ = e.write(map[string]any{
			"jsonrpc": "2.0",
			"id":      *f.ID,
			"result":  map[string]any{"decision": codexRefusal},
		})
		e.emitError("refused " + f.Method + " — approvals are not wired to this backend yet")
		return
	}
	// Not an approval. Answer with the JSON-RPC "method not found" code, which
	// is a well-defined way to say "this client cannot do that" and leaves the
	// server to decide what happens next.
	_ = e.write(map[string]any{
		"jsonrpc": "2.0",
		"id":      *f.ID,
		"error": map[string]any{
			"code":    -32601,
			"message": "cathode does not implement " + f.Method,
		},
	})
	e.emitError("unhandled request " + f.Method)
}

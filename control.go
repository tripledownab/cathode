package main

import (
	"encoding/json"
	"fmt"
	"time"
)

// outControl is the NDJSON envelope for control requests on stdin. Mirrors the
// Agent SDK streaming-input control_request shape; the `claude` CLI honours
// these mid-session, so we can change permission mode, model, etc. without
// restarting the subprocess.
type outControl struct {
	Type      string            `json:"type"`
	RequestID string            `json:"request_id"`
	Request   map[string]string `json:"request"`
}

// sendControl marshals one control_request and writes it to the subprocess
// stdin. prefix only tags the request id (handy in -debug logs); req carries
// the "subtype" plus any subtype-specific fields. These are fire-and-forget:
// the CLI replies with a control_response we don't block on, and an unsupported
// subtype surfaces as a "[remote] … rejected" line under -debug rather than an
// error here.
func (e *Engine) sendControl(prefix string, req map[string]string) error {
	m := outControl{
		Type:      "control_request",
		RequestID: fmt.Sprintf("cathode-%s-%d", prefix, time.Now().UnixNano()),
		Request:   req,
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.stdin.Write(append(b, '\n')); err != nil {
		return err
	}
	debug.Logf("stdin", "%s", b)
	return nil
}

// Initialize runs the streaming-input handshake. The success reply carries the
// session's capability snapshot; we use its model list to populate /model (see
// handleEvent). Safe to send once at startup — it doesn't disturb turns.
func (e *Engine) Initialize() error {
	return e.sendControl("init", map[string]string{"subtype": "initialize"})
}

// (/compact is not a control request — it's a built-in slash command sent as a
// user turn; see the compact command in commands.go.)

// Interrupt asks the running subprocess to abort the current turn. Whether it
// lands depends on what claude was doing when it arrived (it can hit
// mid-tool-call); the UI flips busy off regardless so the prompt comes back.
func (e *Engine) Interrupt() error {
	return e.sendControl("int", map[string]string{"subtype": "interrupt"})
}

// SetPermissionMode switches permission mode mid-session. mode is one of
// "default" | "plan" | "acceptEdits" | "bypassPermissions" — the same values
// --permission-mode takes.
func (e *Engine) SetPermissionMode(mode string) error {
	return e.sendControl("ctrl", map[string]string{"subtype": "set_permission_mode", "mode": mode})
}

// SetModel switches the model for subsequent turns. model is a CLI alias
// ("opus" | "sonnet" | "haiku") or a full model id; "" falls back to the
// account default. The CLI rejects an unknown id with a "[remote] set_model
// rejected" response (visible under -debug), leaving the model unchanged.
func (e *Engine) SetModel(model string) error {
	return e.sendControl("mdl", map[string]string{"subtype": "set_model", "model": model})
}

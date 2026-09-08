// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"time"
)

// codexMsg carries one inbound frame into the Bubble Tea Update loop, the way
// streamMsg does for claude.
type codexMsg struct{ frame codexFrame }

// Synthetic methods cathode raises itself. Namespaced under "cathode/" so they
// can never collide with something the app-server adds later: every real method
// is under a codex-owned prefix, and this makes the distinction checkable
// rather than a matter of memory.
const (
	codexClosedMethod = "cathode/closed" // the subprocess exited
	codexErrorMethod  = "cathode/error"  // a call failed, and nobody was waiting
	codexModelsMethod = "cathode/models" // the model list, fetched not streamed
)

// codexCallTimeout bounds a request that gets no reply. It is generous because
// the only blocking caller is the opening handshake, which starts a subprocess,
// reads config and may refresh an auth token.
const codexCallTimeout = 60 * time.Second

// write marshals one JSON-RPC message and writes the line.
func (e *codexEngine) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	e.wmu.Lock()
	defer e.wmu.Unlock()
	if _, err := e.stdin.Write(append(b, '\n')); err != nil {
		return err
	}
	debug.Logf("stdin", "%s", b)
	return nil
}

// notify sends a fire-and-forget notification (no id, no reply).
func (e *codexEngine) notify(method string, params any) error {
	return e.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// call sends a request and waits for its reply. Blocking, so it belongs in a
// tea.Cmd goroutine or in startup code — never directly in Update.
func (e *codexEngine) call(method string, params any) (json.RawMessage, error) {
	id, ch := e.pending.begin()
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	if err := e.write(msg); err != nil {
		e.pending.abandon(id)
		return nil, err
	}
	select {
	case f := <-ch:
		if f.Error != nil {
			return nil, f.Error
		}
		return f.Result, nil
	case <-time.After(codexCallTimeout):
		e.pending.abandon(id)
		return nil, fmt.Errorf("codex: no reply to %s within %s", method, codexCallTimeout)
	}
}

// fire sends a request without blocking the caller, and surfaces a failure as a
// UI frame instead of returning it. Update runs on the UI timeline, so nothing
// called from it may wait on a round trip.
func (e *codexEngine) fire(method string, params any, onResult func(json.RawMessage)) error {
	id, ch := e.pending.begin()
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	if err := e.write(msg); err != nil {
		e.pending.abandon(id)
		return err
	}
	go func() {
		select {
		case f := <-ch:
			if f.Error != nil {
				e.emitError(method + ": " + f.Error.Error())
				return
			}
			if onResult != nil {
				onResult(f.Result)
			}
		case <-time.After(codexCallTimeout):
			e.pending.abandon(id)
			e.emitError(fmt.Sprintf("%s: no reply within %s", method, codexCallTimeout))
		}
	}()
	return nil
}

// emitError raises a synthetic frame so a failed background call is visible in
// the transcript rather than only in a -debug log.
func (e *codexEngine) emitError(msg string) {
	b, _ := json.Marshal(map[string]string{"message": msg})
	e.emit(codexFrame{Method: codexErrorMethod, Params: b})
}

// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// decode is what the reader does to one line of stdout.
func decode(t *testing.T, line string) codexFrame {
	t.Helper()
	var f codexFrame
	if err := json.Unmarshal([]byte(line), &f); err != nil {
		t.Fatalf("decode %q: %v", line, err)
	}
	return f
}

// The three inbound shapes, taken from a recorded app-server session.
//
// The id:0 case is the one that matters. codex numbers its own requests from
// zero, so a plain int field cannot tell "no id" from "id zero" — every id is a
// pointer for exactly this line. Misread, the first approval of every session
// is filed as a reply nobody waits for, no answer is ever sent, and the turn
// hangs with the pane never opening.
func TestClassifyTellsTheThreeInboundShapesApart(t *testing.T) {
	cases := []struct {
		name string
		line string
		want frameKind
	}{
		{"response", `{"id":1,"result":{"userAgent":"x"}}`, frameResponse},
		{"notification", `{"method":"item/started","params":{}}`, frameNotification},
		{"server request", `{"method":"item/fileChange/requestApproval","id":7,"params":{}}`, frameServerReq},
		{"server request with id zero", `{"method":"item/commandExecution/requestApproval","id":0,"params":{}}`, frameServerReq},
		{"error response", `{"id":2,"error":{"code":-1,"message":"nope"}}`, frameResponse},
	}
	for _, c := range cases {
		if got := decode(t, c.line).classify(); got != c.want {
			t.Errorf("%s: classify = %v, want %v", c.name, got, c.want)
		}
	}
}

// A reply reaches the caller that asked, and only that caller.
func TestPendingDeliversToTheRightWaiter(t *testing.T) {
	p := newCodexPending()
	id1, ch1 := p.begin()
	id2, ch2 := p.begin()
	if id1 == id2 {
		t.Fatalf("ids must be distinct, both were %d", id1)
	}

	p.deliver(codexFrame{ID: &id2, Result: json.RawMessage(`{"ok":true}`)})
	select {
	case f := <-ch2:
		if string(f.Result) != `{"ok":true}` {
			t.Errorf("waiter 2 got %s", f.Result)
		}
	default:
		t.Fatal("waiter 2 got nothing")
	}
	select {
	case <-ch1:
		t.Error("waiter 1 must not receive another call's reply")
	default:
	}
}

// An id nobody waits for is dropped rather than blocking the reader. A call
// that timed out has already gone, and the reader must not stall on it — that
// would freeze every later frame behind one abandoned reply.
func TestPendingDropsRepliesNobodyIsWaitingFor(t *testing.T) {
	p := newCodexPending()
	id, _ := p.begin()
	p.abandon(id)

	done := make(chan struct{})
	go func() { p.deliver(codexFrame{ID: &id}); close(done) }()
	<-done // a stall here means the reader would stall too
}

// When the subprocess dies, every in-flight call is woken with an error. Left
// alone each one waits out its own timeout, and the UI sits frozen meanwhile.
func TestPendingFailAllWakesEveryCaller(t *testing.T) {
	p := newCodexPending()
	_, ch1 := p.begin()
	_, ch2 := p.begin()

	p.failAll(errEngineClosed)

	for i, ch := range []chan codexFrame{ch1, ch2} {
		select {
		case f := <-ch:
			if f.Error == nil {
				t.Errorf("waiter %d woke with no error", i+1)
			}
		default:
			t.Errorf("waiter %d was left hanging", i+1)
		}
	}
}

// The whole-name rule, the same one TestScrubbedEnvDropsWhatClaudeMustNotInherit
// pins for claude: a substring test also drops any variable whose name merely
// ends with one of these.
func TestCodexEnvDropsWholeNamesOnly(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-not-real")
	t.Setenv("OPENAI_BASE_URL", "http://example.invalid")
	t.Setenv("API_KEY", "keep-me")
	t.Setenv("BASE_URL", "keep-me-too")
	t.Setenv("OPENAI_API_KEY_BACKUP", "keep-me-three")

	got := map[string]bool{}
	for _, kv := range codexEnv() {
		if name, _, ok := strings.Cut(kv, "="); ok {
			got[name] = true
		}
	}
	for _, name := range []string{"OPENAI_API_KEY", "OPENAI_BASE_URL"} {
		if got[name] {
			t.Errorf("%s must not reach codex", name)
		}
	}
	for _, name := range []string{"API_KEY", "BASE_URL", "OPENAI_API_KEY_BACKUP"} {
		if !got[name] {
			t.Errorf("%s is not one of the dropped names and must survive", name)
		}
	}
	if len(got) == 0 {
		t.Fatal("codexEnv returned nothing")
	}
	if _, ok := os.LookupEnv("PATH"); ok && !got["PATH"] {
		t.Error("PATH must survive, or the subprocess cannot find its own tools")
	}
}

// Each cathode mode states both codex knobs. An unknown mode must land on the
// gated pair, never a quieter one.
func TestCodexPolicyForModeStatesBothKnobs(t *testing.T) {
	cases := []struct{ mode, policy, sandbox string }{
		{"plan", "untrusted", "read-only"},
		{"ask", "untrusted", "workspace-write"},
		{"build", "on-request", "workspace-write"},
		{"bypass", "never", "danger-full-access"},
		{"", "untrusted", "workspace-write"},
		{"nonsense", "untrusted", "workspace-write"},
	}
	for _, c := range cases {
		policy, sandbox := codexPolicyForMode(c.mode)
		if policy != c.policy || sandbox != c.sandbox {
			t.Errorf("%q: got %s/%s, want %s/%s", c.mode, policy, sandbox, c.policy, c.sandbox)
		}
	}
}

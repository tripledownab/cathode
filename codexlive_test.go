// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"testing"
	"time"
)

// A live check against the real `codex` CLI, off by default because it spends a
// turn on the user's subscription and needs `codex login` to have been run:
//
//	CATHODE_CODEX_LIVE=1 go test -run TestCodexLive ./...
//
// Same opt-in shape as the asset generators. It exists because the scripted
// stand-in in codexengine_test.go proves the framing cathode expects, not the
// framing codex actually sends, and those are different claims. This one asks
// the second question.
func TestCodexLiveRoundTrip(t *testing.T) {
	if os.Getenv("CATHODE_CODEX_LIVE") == "" {
		t.Skip("set CATHODE_CODEX_LIVE=1 to run against the real codex CLI")
	}

	e, err := newCodexEngine(codexEngineConfig{Mode: "plan", Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer e.Close()

	frames := make(chan codexFrame, 256)
	e.mu.Lock()
	e.sink = func(f codexFrame) { frames <- f }
	e.mu.Unlock()

	if err := e.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	e.mu.Lock()
	thread := e.threadID
	e.mu.Unlock()
	if thread == "" {
		t.Fatal("no thread id after Initialize")
	}
	t.Logf("thread %s", thread)

	if err := e.Send("Reply with exactly: OK. Do not use any tools."); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Drain until the turn ends, collecting what the adapter would render.
	m, _ := newTestModel(t, "")
	deadline := time.After(3 * time.Minute)
	for {
		select {
		case f := <-frames:
			m.handleCodexEvent(f)
			if f.Method == codexErrorMethod || f.Method == "error" {
				t.Fatalf("codex reported an error: %s", f.Params)
			}
			if f.Method == "turn/completed" || f.Method == "turn/failed" {
				for _, e := range m.entries {
					t.Logf("entry kind=%d text=%q", e.kind, e.text)
				}
				if m.busy {
					t.Error("busy should be cleared when the turn ends")
				}
				return
			}
		case <-deadline:
			t.Fatal("no turn/completed within the deadline")
		}
	}
}

// What each mode actually does with a gated action, against the real CLI.
//
// The property that matters is that the turn always ENDS. codex blocks until a
// server request is answered, so the failure guarded against is not a wrong
// answer, it is no answer — which presents as a frozen UI with nothing in the
// log.
//
// The two rows also pin the current limit of this backend. In build mode codex
// asks for nothing and the action runs, so the backend is usable today. In ask
// mode every gated action is refused, because the approval pane is not wired to
// codex yet and granting silently would defeat the mode.
func TestCodexLiveGatedActionsAlwaysEndTheTurn(t *testing.T) {
	if os.Getenv("CATHODE_CODEX_LIVE") == "" {
		t.Skip("set CATHODE_CODEX_LIVE=1 to run against the real codex CLI")
	}
	for _, c := range []struct {
		mode        string
		wantRefusal bool
	}{
		{"build", false},
		{"ask", true},
	} {
		t.Run(c.mode, func(t *testing.T) {
			e, err := newCodexEngine(codexEngineConfig{Mode: c.mode, Cwd: t.TempDir()})
			if err != nil {
				t.Fatalf("spawn: %v", err)
			}
			defer e.Close()

			frames := make(chan codexFrame, 256)
			e.mu.Lock()
			e.sink = func(f codexFrame) { frames <- f }
			e.mu.Unlock()

			if err := e.Initialize(); err != nil {
				t.Fatalf("Initialize: %v", err)
			}
			if err := e.Send("Create a file called probe.txt containing the word hello."); err != nil {
				t.Fatalf("Send: %v", err)
			}

			var refused bool
			deadline := time.After(3 * time.Minute)
			for {
				select {
				case f := <-frames:
					if f.Method == codexErrorMethod {
						t.Logf("notice: %s", f.Params)
						refused = true
					}
					if f.Method == "turn/completed" || f.Method == "turn/failed" {
						if refused != c.wantRefusal {
							t.Errorf("%s mode: refused=%v, want %v", c.mode, refused, c.wantRefusal)
						}
						return
					}
				case <-deadline:
					t.Fatal("the turn never ended — a server request went unanswered")
				}
			}
		})
	}
}

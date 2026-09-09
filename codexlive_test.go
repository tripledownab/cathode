// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	sinkTo(e, frames, nil)

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

// What each mode does with a gated action, against the real CLI.
//
// The property that matters is that the turn always ENDS. codex blocks until a
// server request is answered, so the failure guarded against is not a wrong
// answer, it is no answer — which presents as a frozen UI with nothing in the
// log.
//
// build asks for nothing and the action runs. ask raises a real approval, which
// this answers the way the pane would.
func TestCodexLiveGatedActionsAlwaysEndTheTurn(t *testing.T) {
	if os.Getenv("CATHODE_CODEX_LIVE") == "" {
		t.Skip("set CATHODE_CODEX_LIVE=1 to run against the real codex CLI")
	}
	for _, c := range []struct {
		mode       string
		wantPrompt bool
		allow      bool
	}{
		{"build", false, false},
		{"ask", true, true},
	} {
		t.Run(c.mode, func(t *testing.T) {
			dir := t.TempDir()
			e, err := newCodexEngine(codexEngineConfig{Mode: c.mode, Cwd: dir})
			if err != nil {
				t.Fatalf("spawn: %v", err)
			}
			defer e.Close()

			frames := make(chan codexFrame, 256)
			other := make(chan tea.Msg, 16)
			sinkTo(e, frames, other)

			if err := e.Initialize(); err != nil {
				t.Fatalf("Initialize: %v", err)
			}
			if err := e.Send("Create a file called probe.txt containing the word hello."); err != nil {
				t.Fatalf("Send: %v", err)
			}

			var asked bool
			deadline := time.After(3 * time.Minute)
			for {
				select {
				case msg := <-other:
					pa, ok := msg.(pendingApprovalMsg)
					if !ok {
						continue
					}
					asked = true
					t.Logf("approval asked: %s", pa.req.toolName)
					pa.req.reply <- approvalReply{allow: c.allow}
				case f := <-frames:
					if f.Method != "turn/completed" && f.Method != "turn/failed" {
						continue
					}
					if asked != c.wantPrompt {
						t.Errorf("%s mode: asked=%v, want %v", c.mode, asked, c.wantPrompt)
					}
					if c.allow {
						if _, err := os.Stat(filepath.Join(dir, "probe.txt")); err != nil {
							t.Errorf("approved, but the file was not written: %v", err)
						}
					}
					return
				case <-deadline:
					t.Fatal("the turn never ended — a server request went unanswered")
				}
			}
		})
	}
}

// A real edit by the real CLI must reach the screen as a diff card.
//
// The unit tests use a hunk copied from a recorded session, which proves
// cathode parses what it was told to expect. This proves codex still sends it.
func TestCodexLiveEditRendersAsADiffCard(t *testing.T) {
	if os.Getenv("CATHODE_CODEX_LIVE") == "" {
		t.Skip("set CATHODE_CODEX_LIVE=1 to run against the real codex CLI")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("line one\nline two\nline three\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// build mode: codex runs the edit without asking, so the turn completes.
	e, err := newCodexEngine(codexEngineConfig{Mode: "build", Cwd: dir})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer e.Close()

	frames := make(chan codexFrame, 256)
	sinkTo(e, frames, nil)

	if err := e.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := e.Send("In target.txt, change the word two to TWO. Edit the file, nothing else."); err != nil {
		t.Fatalf("Send: %v", err)
	}

	m, _ := newTestModel(t, "")
	deadline := time.After(3 * time.Minute)
	for {
		select {
		case f := <-frames:
			m.handleCodexEvent(f)
			if f.Method == "turn/completed" || f.Method == "turn/failed" {
				for _, en := range m.entries {
					if en.kind != entDiff {
						continue
					}
					card := stripANSI(renderDiffFor(diffUnified, en.diffs[0], 80))
					t.Logf("diff card:\n%s", card)
					if !strings.Contains(card, "TWO") {
						t.Errorf("the card should show the edit, got:\n%s", card)
					}
					return
				}
				t.Error("the edit never produced a diff entry")
				return
			}
		case <-deadline:
			t.Fatal("the turn never ended")
		}
	}
}

// The /model picker must offer codex's own models, not claude's aliases.
// model/list is a request, so the list reaches the UI as a fetched frame; this
// checks that round trip against the real catalogue.
func TestCodexLiveModelListReachesThePicker(t *testing.T) {
	if os.Getenv("CATHODE_CODEX_LIVE") == "" {
		t.Skip("set CATHODE_CODEX_LIVE=1 to run against the real codex CLI")
	}
	e, err := newCodexEngine(codexEngineConfig{Mode: "plan", Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer e.Close()

	frames := make(chan codexFrame, 64)
	sinkTo(e, frames, nil)

	if err := e.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	m, _ := newTestModel(t, "")
	m.backend = backendCodex
	deadline := time.After(30 * time.Second)
	for {
		select {
		case f := <-frames:
			m.handleCodexEvent(f)
			if f.Method != codexModelsMethod {
				continue
			}
			items := m.modelItems()
			if len(items) == 0 {
				t.Fatal("the model frame arrived but the picker is empty")
			}
			for _, it := range items {
				t.Logf("model row: %s — %s", it.title, it.subtitle)
				switch it.id {
				case "opus", "sonnet", "haiku":
					t.Errorf("codex picker offers claude's %q", it.id)
				case "":
					t.Error("a row with no id cannot be selected")
				}
			}
			return
		case <-deadline:
			t.Fatal("no model list within the deadline")
		}
	}
}

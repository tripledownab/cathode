// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// switchable builds a model that can actually perform a swap: a program handle
// to pipe into, and a codex stand-in so no real CLI is spawned.
func switchable(t *testing.T, backend string) (model, *fakeEngine) {
	t.Helper()
	fakeAppServer(t, echoServer)
	m, f := newTestModel(t, "")
	m.backend = backend
	m.prog = &progRef{}
	m.prog.set(tea.NewProgram(model{}))
	return m, f
}

// Every declined switch says why. A silent return reads as "switched" and is
// not — the same rule commitSysPrompt follows.
func TestBackendSwitchDeclinesWithAReason(t *testing.T) {
	cases := []struct {
		name, to, want string
		busy           bool
		noProg         bool
	}{
		{"same backend", backendClaude, "already on claude", false, false},
		{"unknown", "gemini", "unknown backend", false, false},
		{"mid turn", backendCodex, "busy", true, false},
		{"before the UI runs", backendCodex, "before the UI is running", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, f := switchable(t, backendClaude)
			m.busy = c.busy
			if c.noProg {
				m.prog = &progRef{}
			}
			before := m.engine

			if cmd := m.commitBackend(c.to); cmd != nil {
				t.Error("a declined switch must not return a command")
			}
			if m.engine != before {
				t.Error("the engine was swapped despite declining")
			}
			if m.backend != backendClaude {
				t.Errorf("backend = %q, want it unchanged", m.backend)
			}
			if f.shutdown {
				t.Error("the old engine was closed despite declining")
			}
			last := m.entries[len(m.entries)-1]
			if !strings.Contains(last.text, c.want) {
				t.Errorf("entry = %q, want it to mention %q", last.text, c.want)
			}
		})
	}
}

// A successful switch replaces the engine, keeps the transcript, and clears
// everything that described the session that is gone.
func TestBackendSwitchResetsSessionStateButKeepsTheTranscript(t *testing.T) {
	m, _ := switchable(t, backendClaude)
	m.add(entUser, "something I asked earlier")
	kept := len(m.entries)

	m.session = "claude-session"
	m.modelID = "opus"
	m.models = []ModelChoice{{Value: "opus"}}
	m.mcpServers = []MCPServerInfo{{Name: "x"}}
	m.ctxTokens = 1234
	m.ctxLimit = 1_000_000
	m.baseCtxLimit = 200_000
	before := m.engine

	if cmd := m.commitBackend(backendCodex); cmd == nil {
		t.Fatal("a real switch should return the new engine's startup commands")
	}

	if m.engine == before {
		t.Error("the engine was not replaced")
	}
	if m.backend != backendCodex {
		t.Errorf("backend = %q, want codex", m.backend)
	}
	if len(m.entries) <= kept {
		t.Error("the transcript should survive a switch")
	}
	if m.entries[kept-1].text != "something I asked earlier" {
		t.Error("an earlier entry was lost")
	}
	last := m.entries[len(m.entries)-1]
	if !strings.Contains(last.text, "does not carry over") {
		t.Errorf("the switch must say the conversation is not carried, got %q", last.text)
	}

	// State belonging to the session that ended.
	for name, got := range map[string]any{
		"session": m.session, "modelID": m.modelID,
		"models": len(m.models), "mcpServers": len(m.mcpServers),
		"ctxTokens": m.ctxTokens,
	} {
		switch v := got.(type) {
		case string:
			if v != "" {
				t.Errorf("%s = %q, want it cleared", name, v)
			}
		case int:
			if v != 0 {
				t.Errorf("%s = %d, want it cleared", name, v)
			}
		}
	}
	if m.ctxLimit != 200_000 {
		t.Errorf("ctxLimit = %d, want the -ctx floor back", m.ctxLimit)
	}
	if !strings.Contains(m.input.Placeholder, "codex") {
		t.Errorf("placeholder = %q, want it to name the new backend", m.input.Placeholder)
	}
}

// A resumed session's id belongs to the backend that issued it. Carrying the
// launch --resume across would hand claude an id codex made up.
func TestWithoutResumeStripsBothFlagForms(t *testing.T) {
	got := withoutResume([]string{"--settings", "{}", "--resume=abc", "-x"})
	if strings.Join(got, " ") != "--settings {} -x" {
		t.Errorf("joined form: got %v", got)
	}
	got = withoutResume([]string{"--resume", "abc", "--verbose"})
	if strings.Join(got, " ") != "--verbose" {
		t.Errorf("separated form: got %v", got)
	}
	orig := []string{"--resume=abc"}
	_ = withoutResume(orig)
	if len(orig) != 1 {
		t.Error("the caller's slice must not be edited underneath it")
	}
}

// The engine we switched away from is still draining into the same program.
// Its trailing frames, and the EOF that follows, must not land in the new
// session — the EOF in particular would announce that the session had ended.
func TestStaleBackendFramesAreIgnoredAfterASwitch(t *testing.T) {
	m, _ := newTestModel(t, "")
	m.backend = backendCodex
	before := len(m.entries)

	// claude traffic arriving after a switch to codex.
	step := func(msg tea.Msg) {
		t.Helper()
		next, _ := m.Update(msg)
		nm, ok := next.(model)
		if !ok {
			t.Fatalf("Update returned %T", next)
		}
		m = nm
	}
	step(streamMsg{env: Envelope{
		Type:    "assistant",
		Message: &APIMessage{Content: []ContentBlock{{Type: "text", Text: "from the old engine"}}},
	}})
	step(streamClosedMsg{})

	if len(m.entries) != before {
		t.Errorf("the old backend added %d entries after the switch", len(m.entries)-before)
	}
	for _, e := range m.entries {
		if strings.Contains(e.text, "session ended") {
			t.Error("the old engine's EOF announced the end of the new session")
		}
	}

	// The backend we are actually on still gets through.
	step(codexMsg{frame: codexFrame{
		Method: "item/completed",
		Params: []byte(`{"item":{"type":"agentMessage","id":"m1","text":"from the live engine"}}`),
	}})
	last := m.entries[len(m.entries)-1]
	if last.text != "from the live engine" {
		t.Errorf("the live backend was filtered out too: %+v", last)
	}
}

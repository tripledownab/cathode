// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Every user-visible mention of the agent follows the backend. These four sites
// each hardcoded "claude", so a codex session was answered by something the
// whole screen called claude.
func TestAgentLabelsFollowTheBackend(t *testing.T) {
	for _, c := range []struct{ backend, want string }{
		{backendClaude, "claude"},
		{backendCodex, "codex"},
		{"", "claude"}, // unset means the default backend, not a blank label
	} {
		if got := agentName(c.backend); got != c.want {
			t.Errorf("agentName(%q) = %q, want %q", c.backend, got, c.want)
		}
		if got := promptPlaceholder(c.backend); !strings.Contains(got, "Ask "+c.want) {
			t.Errorf("placeholder for %q = %q, want it to name %q", c.backend, got, c.want)
		}
		if got := agentTagline(c.backend); !strings.Contains(got, c.want) {
			t.Errorf("tagline for %q = %q, want it to name %q", c.backend, got, c.want)
		}
	}
	// The taglines name different plans, so one is not silently reused.
	if agentTagline(backendClaude) == agentTagline(backendCodex) {
		t.Error("both backends share a tagline; each rides a different plan")
	}
}

// The placeholder is set once when the prompt is built, so a model constructed
// for codex must not carry claude's.
func TestPromptPlaceholderComesFromTheLaunchConfig(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	m := newModel(launchConfig{Engine: &fakeEngine{}, Backend: backendCodex, Mode: "ask", Spinner: "bar"})
	if got := m.input.Placeholder; !strings.Contains(got, "Ask codex") {
		t.Errorf("placeholder = %q, want it to name codex", got)
	}
}

// A backend's aliases are meaningless to another one. Offering "opus" on codex
// is not a harmless default: it is a row that cannot work.
func TestModelFallbackNeverOffersAnotherBackendsModels(t *testing.T) {
	claude := fallbackModelItems(backendClaude)
	if len(claude) != 3 || claude[0].id != "opus" {
		t.Errorf("claude fallback = %+v, want its three aliases", claude)
	}
	for _, it := range fallbackModelItems(backendCodex) {
		switch it.id {
		case "opus", "sonnet", "haiku":
			t.Errorf("codex fallback offers claude's %q", it.id)
		}
	}
}

// The live list replaces the fallback. codex sends it as a fetched frame rather
// than on the stream, because model/list is a request.
func TestCodexModelsFrameFillsThePicker(t *testing.T) {
	m, _ := newTestModel(t, "")
	m.backend = backendCodex

	if got := m.modelItems(); len(got) == 0 || got[0].id != "" {
		t.Fatalf("before the list arrives, want the placeholder row, got %+v", got)
	}

	models := []ModelChoice{{Value: "gpt-x", DisplayName: "GPT-X", Description: "the one"}}
	b, err := json.Marshal(models)
	if err != nil {
		t.Fatal(err)
	}
	m.handleCodexEvent(codexFrame{Method: codexModelsMethod, Params: b})

	items := m.modelItems()
	if len(items) != 1 || items[0].id != "gpt-x" || items[0].title != "GPT-X" {
		t.Errorf("picker rows = %+v, want the reported model", items)
	}
}

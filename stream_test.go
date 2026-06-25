package main

import "testing"

// Thinking blocks render as their own (dim) entry; empty ones are skipped.
func TestThinkingBlocks(t *testing.T) {
	m := &model{}
	m.handleEvent(Envelope{Type: "assistant", Message: &APIMessage{Content: []ContentBlock{
		{Type: "thinking", Thinking: "let me reason about this"},
		{Type: "text", Text: "the answer is 42"},
	}}})
	if len(m.entries) != 2 {
		t.Fatalf("want a thinking + a claude entry, got %d", len(m.entries))
	}
	if m.entries[0].kind != entThinking || m.entries[1].kind != entClaude {
		t.Fatalf("kinds: got %v,%v want thinking,claude", m.entries[0].kind, m.entries[1].kind)
	}

	m2 := &model{}
	m2.handleEvent(Envelope{Type: "assistant", Message: &APIMessage{Content: []ContentBlock{
		{Type: "thinking", Thinking: "  "}, // empty after trim → skipped
		{Type: "text", Text: "hi"},
	}}})
	if len(m2.entries) != 1 || m2.entries[0].kind != entClaude {
		t.Fatalf("empty thinking should be skipped, got %d entries", len(m2.entries))
	}
}

// Only failing/blocking hooks surface; routine successes stay silent.
func TestHookResponseSurfacing(t *testing.T) {
	ok := &model{}
	ok.handleEvent(Envelope{Type: "system", Subtype: "hook_response", HookName: "SessionStart", ExitCode: 0, Outcome: "success"})
	if len(ok.entries) != 0 {
		t.Errorf("successful hook should be silent, got %d entries", len(ok.entries))
	}

	blocked := &model{}
	blocked.handleEvent(Envelope{Type: "system", Subtype: "hook_response", HookName: "PreToolUse", ExitCode: 2, Outcome: "block", Stderr: "nope"})
	if len(blocked.entries) != 1 || blocked.entries[0].kind != entError {
		t.Fatalf("blocking hook should surface as an error entry, got %d", len(blocked.entries))
	}
}

// The context gauge follows assistant usage, and a successful /compact must drop
// it immediately rather than leaving it stuck at the pre-compact level (the
// post-compact size isn't reported until the next turn).
func TestCompactResetsContextGauge(t *testing.T) {
	m := &model{ctxLimit: 200000}

	m.handleEvent(Envelope{Type: "assistant", Message: &APIMessage{
		Usage: &Usage{InputTokens: 50000, CacheReadInputTokens: 100000},
	}})
	if m.ctxTokens != 150000 {
		t.Fatalf("assistant usage: ctxTokens=%d want 150000", m.ctxTokens)
	}

	m.handleEvent(Envelope{Type: "system", Subtype: "status", CompactResult: "success"})
	if m.ctxTokens != 0 {
		t.Fatalf("after compact: ctxTokens=%d want 0 (gauge should drop, not stick)", m.ctxTokens)
	}

	// The next reply's usage refines the gauge to the real post-compact size.
	m.handleEvent(Envelope{Type: "assistant", Message: &APIMessage{
		Usage: &Usage{InputTokens: 12000},
	}})
	if m.ctxTokens != 12000 {
		t.Fatalf("post-compact turn: ctxTokens=%d want 12000", m.ctxTokens)
	}
}

package main

import "testing"

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

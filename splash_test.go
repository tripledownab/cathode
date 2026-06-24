package main

import (
	"strings"
	"testing"
)

// TestSplashRendersBeforeWindowSizeMsg pins the fix for the "starting…" hang:
// View() must paint the splash on the very first frame, before any
// tea.WindowSizeMsg arrives. A regression here would re-introduce the original
// symptom where ./doorway -mode ask appears to hang at "starting…" on first
// run, while the engine is healthy.
func TestSplashRendersBeforeWindowSizeMsg(t *testing.T) {
	m := newModel(&Engine{}, "ask", nil, "bar", "")
	// Fast-forward the reveal so the modem-handshake markers are present.
	// The test isn't about the animation; it's about the "starting…"
	// placeholder never leaking through before WindowSizeMsg.
	m.splashFrame = splashFinalFrame
	out := m.View()
	if strings.Contains(out, "starting…") {
		t.Fatalf("View() returned the not-ready placeholder before WindowSizeMsg:\n%s", out)
	}
	// "ATDT" is rendered via cDim only — no leet/studly mangling — so it's a
	// stable marker that the splash screen actually painted.
	if !strings.Contains(out, "ATDT") {
		t.Fatalf("View() did not render splash content (no ATDT marker):\n%s", out)
	}
}

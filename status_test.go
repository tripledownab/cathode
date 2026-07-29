package main

import (
	"strings"
	"testing"
)

// A recent Ctrl+C (armed) swaps the right-hand READY/WORKING indicator for the
// "again to exit" hint, so the user sees that a second press quits.
func TestStatusShowsCtrlCExitHint(t *testing.T) {
	ready := bbsStatus("ask", "opus", "sess", "", 0, 0, 0, 200_000, false, false, "", 80)
	if !strings.Contains(ready, "READY") || strings.Contains(ready, "AGAIN TO EXIT") {
		t.Errorf("unarmed status should show READY, not the exit hint:\n%s", ready)
	}
	armed := bbsStatus("ask", "opus", "sess", "", 0, 0, 0, 200_000, false, true, "", 80)
	if !strings.Contains(armed, "AGAIN TO EXIT") || strings.Contains(armed, "READY") {
		t.Errorf("armed status should show the exit hint, not READY:\n%s", armed)
	}
}

func TestShortModel(t *testing.T) {
	cases := map[string]string{
		"claude-sonnet-4-5-20250929": "sonnet",
		"claude-opus-4-1-20250805":   "opus",
		"claude-3-5-haiku-20241022":  "haiku",
		"opus[1m]":                   "opus",
		"Opus (1M context)":          "opus",
		"Default (recommended)":      "default", // no family word → first token
		"":                           "default",
		"Frankenmodel X":             "frankenmodel",
	}
	for in, want := range cases {
		if got := shortModel(in); got != want {
			t.Errorf("shortModel(%q) = %q, want %q", in, got, want)
		}
	}
}

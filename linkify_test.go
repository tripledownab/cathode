package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLinkify(t *testing.T) {
	// No URL: returned untouched (the fast path the asset-gen relies on).
	if got := linkify("just some text, no links"); got != "just some text, no links" {
		t.Errorf("non-URL text changed: %q", got)
	}

	// A URL gets wrapped in OSC 8 with the URL as both target and visible text.
	out := linkify("docs at https://example.com/x here")
	want := "docs at " + oscOpen + "https://example.com/x" + oscClose +
		"https://example.com/x" + oscOpen + oscClose + " here"
	if out != want {
		t.Errorf("linkify mismatch:\n got %q\nwant %q", out, want)
	}

	// Trailing prose punctuation stays outside the link target.
	p := linkify("see https://example.com.")
	if !strings.Contains(p, oscOpen+"https://example.com"+oscClose) || !strings.HasSuffix(p, ".") {
		t.Errorf("trailing punctuation handling: %q", p)
	}

	// Zero-width: linkified text has the same visible width as the original.
	plain := "a https://example.com/path b"
	if lipgloss.Width(linkify(plain)) != lipgloss.Width(plain) {
		t.Errorf("OSC 8 changed visible width")
	}
}

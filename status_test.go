package main

import "testing"

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

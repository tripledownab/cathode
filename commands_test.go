package main

import "testing"

// paletteItems merges our in-process commands with claude's reported commands
// (skills + plugins), deduped by name with ours winning.
func TestPaletteItemsMerge(t *testing.T) {
	m := &model{commands: []CommandInfo{
		{Name: "clear", Description: "claude's clear (should be shadowed)"},
		{Name: "deep-research", Description: "a skill"},
		{Name: "stripe:test-cards", Description: "a plugin command"},
	}}
	count := map[string]int{}
	var clearSub string
	for _, it := range m.paletteItems() {
		count[it.id]++
		if it.id == "clear" {
			clearSub = it.subtitle
		}
	}
	if count["clear"] != 1 {
		t.Errorf("clear should appear once (ours wins), got %d", count["clear"])
	}
	if clearSub != "clear the transcript" {
		t.Errorf("clear should keep OUR description, got %q", clearSub)
	}
	if count["deep-research"] != 1 {
		t.Error("skill command should be listed in the palette")
	}
	if count["stripe:test-cards"] != 1 {
		t.Error("plugin command should be listed in the palette")
	}
}

// runSlash handles our own commands in-process and reports not-handled for
// anything else (without erroring), so handleEnter forwards it to claude.
func TestRunSlashForwarding(t *testing.T) {
	// A command we own is handled.
	if _, _, handled := runSlash(&model{}, "/help"); !handled {
		t.Error("/help should be handled in-process")
	}

	// An unknown command is NOT handled and adds no error entry — the caller
	// forwards the line to claude verbatim.
	m := &model{}
	nm, _, handled := runSlash(m, "/frobnicate --foo")
	if handled {
		t.Error("unknown /command should report not-handled so it gets forwarded")
	}
	if len(nm.entries) != 0 {
		t.Errorf("unknown /command should not add a transcript entry, got %d", len(nm.entries))
	}

	// A non-slash line is not a command.
	if _, _, handled := runSlash(&model{}, "just a prompt"); handled {
		t.Error("a plain prompt should not be treated as a command")
	}
}

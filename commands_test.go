package main

import "testing"

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

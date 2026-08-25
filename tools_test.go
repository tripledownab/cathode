// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// TestUnwrapToolError pins the rule: a body that is nothing but the CLI's
// <tool_use_error> wrapper loses the tag, and anything else keeps its text
// byte for byte.
func TestUnwrapToolError(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"wrapped", "<tool_use_error>File has been modified since read.</tool_use_error>",
			"File has been modified since read."},
		{"wrapped with slack", "  <tool_use_error>boom</tool_use_error>\n", "boom"},
		{"plain", "exit status 1", "exit status 1"},
		{"quoted mid sentence", "claude prints <tool_use_error> around failures",
			"claude prints <tool_use_error> around failures"},
		{"two wrappers", "<tool_use_error>a</tool_use_error> and <tool_use_error>b</tool_use_error>",
			"<tool_use_error>a</tool_use_error> and <tool_use_error>b</tool_use_error>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := unwrapToolError(c.in); got != c.want {
				t.Fatalf("unwrapToolError(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestRenderToolResultHidesErrorTag checks the card the user actually sees:
// the failure message stays, the markup goes.
func TestRenderToolResultHidesErrorTag(t *testing.T) {
	out := renderToolResult("Edit", "<tool_use_error>Read it again.</tool_use_error>", true, 80)
	if strings.Contains(out, "tool_use_error") {
		t.Errorf("card still shows the wrapper tag:\n%s", out)
	}
	if !strings.Contains(out, "Read it again.") {
		t.Errorf("card lost the error message:\n%s", out)
	}
}

// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import "strings"

// ---- what the running backend is called on screen ----
//
// Every user-visible mention of the agent goes through here. Before this, the
// name was written out at each site — the prompt placeholder, the reply label,
// the approval bar, the header — and adding a second backend left all four
// saying "claude" while codex was answering.
//
// One function rather than four constants, so a third backend is a case in one
// switch instead of a hunt for string literals. Anything that still hardcodes a
// name is either about claude specifically (its config dir, its CLI flags) or a
// bug.

// agentName is the backend's name as the transcript and prompt refer to it.
// Lowercase: the chrome applies its own casing (studly, leet) on top.
func agentName(backend string) string {
	if backend == backendCodex {
		return "codex"
	}
	return "claude"
}

// agentTagline is the header's subtitle. It names the plan each backend rides,
// because riding a subscription rather than an API key is the point of the
// program and the header is where that is said.
func agentTagline(backend string) string {
	if backend == backendCodex {
		return "codex on your ChatGPT plan"
	}
	return "claude on your Max plan"
}

// agentDialString is the splash screen's dial-up line. Pure BBS flavour — ATDT
// was the Hayes modem command to dial — but it names the agent, so it belongs
// with the rest of the copy that does. Upper case because a 1980s board would
// have printed it that way, and because the number is a joke about the name.
func agentDialString(backend string) string {
	return "ATDT 1-800-" + strings.ToUpper(agentName(backend)) + " . . ."
}

// promptPlaceholder is the empty-input hint.
func promptPlaceholder(backend string) string {
	return "Ask " + agentName(backend) + "…  (enter sends · alt+enter / ctrl+j / \\↵ for a new line)"
}

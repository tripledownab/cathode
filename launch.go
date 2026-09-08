// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

// launchConfig is everything main resolved before the UI exists.
//
// A struct rather than a parameter list. The list had reached six positional
// arguments with four adjacent strings, where transposing two is silent: the
// compiler cannot tell a mode from a spinner name from a session id. Naming
// them at the call site makes that mistake impossible to write, and it means a
// new setting adds a field instead of shifting every caller's arguments along.
//
// SysPrompt is the one field with a rule attached. It is the standing-
// instruction text the live subprocess actually launched with, handed over by
// sysPromptArgs rather than re-read from disk, and it is empty whenever no
// style reached the agent — see sysprompt.go for why those are different
// questions.
type launchConfig struct {
	Engine    Engine
	Backend   string     // claude | codex; names the agent on screen (agentname.go)
	Approvals *Approvals // nil when nothing is gated (bypass mode)
	Mode      string     // ask | plan | build | bypass
	Spinner   string     // throbber style id
	ResumeID  string     // session to replay into the transcript, or ""
	SysPrompt string     // standing instructions in force, or ""
}

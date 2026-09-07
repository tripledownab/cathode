// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

// codexPolicyForMode maps a cathode mode onto the two knobs codex actually has:
// an approval policy and a sandbox.
//
// claude expresses this as one value (--permission-mode); codex splits it in
// two, and the split is the interesting part. "What may run without asking" and
// "what may be touched at all" are independent there, so a mode has to state
// both or it inherits a default nobody chose.
//
// The pairs, and why:
//
//	plan    read-only + untrusted   nothing is written, and anything that would
//	                                run is asked about first.
//	ask     workspace-write + untrusted
//	                                edits are allowed but every gated action
//	                                surfaces in the approval pane. This is the
//	                                mode the pane exists for.
//	build   workspace-write + on-request
//	                                the agent proceeds and asks only when it
//	                                judges it needs to — the closest thing codex
//	                                has to acceptEdits.
//	bypass  danger-full-access + never
//	                                nothing is gated. Named to match what it
//	                                does, and deliberately off the Shift+Tab
//	                                wheel (see nextMode).
//
// Anything unrecognised gets the ask pair. An unknown mode must not be quieter
// than the modes we know: defaulting to the gated one fails safe.
//
// The sandbox returned here is thread/start's `sandbox`, a SandboxMode string.
// turn/start takes the same concept as `sandboxPolicy`, a tagged object with a
// DIFFERENT spelling — codexSandboxPolicy converts. Do not pass one where the
// other belongs: the app-server rejects the frame, turn/start never replies
// with a turn, and the session simply never starts one.
func codexPolicyForMode(mode string) (policy, sandbox string) {
	switch mode {
	case "plan":
		return "untrusted", "read-only"
	case "build":
		return "on-request", "workspace-write"
	case "bypass":
		return "never", "danger-full-access"
	default: // "ask", and anything unknown
		return "untrusted", "workspace-write"
	}
}

// codexSandboxPolicy converts a SandboxMode string into the tagged object
// turn/start wants.
//
// The two vocabularies are genuinely different, not merely styled differently:
// thread/start says "read-only", turn/start says {"type":"readOnly"}. Writing
// the conversion down once, here, is what stops the next caller guessing — the
// first version of Send guessed {"mode": "read-only"} and every turn silently
// failed to start.
func codexSandboxPolicy(sandbox string) map[string]any {
	switch sandbox {
	case "read-only":
		return map[string]any{"type": "readOnly"}
	case "danger-full-access":
		return map[string]any{"type": "dangerFullAccess"}
	default: // "workspace-write"
		return map[string]any{"type": "workspaceWrite"}
	}
}

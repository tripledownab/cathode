// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import tea "github.com/charmbracelet/bubbletea"

// Engine is the seam between the UI and the agent subprocess it drives.
//
// Everything above this line — the transcript, the diff cards, the approval
// pane, the pickers — works in terms of `entry` values and knows nothing about
// the wire format underneath. Keeping that true is what lets the UI be tested
// without spawning a real subprocess, and what keeps one protocol's shape from
// leaking into a hundred call sites.
//
// The methods are the whole vocabulary the UI needs: one to open a turn, one to
// abort it, two to change settings mid-session, one handshake, plus the
// lifecycle pair main owns (Pipe and Close).
//
// Close has the ordering constraint documented on claudeEngine.Close: main
// calls it AFTER the Bubble Tea program returns, never from the Update loop.
type Engine interface {
	// Send writes one user turn.
	Send(text string) error
	// Initialize runs the startup handshake that reports session capabilities.
	Initialize() error
	// Interrupt asks the subprocess to abort the turn in flight.
	Interrupt() error
	// SetPermissionMode switches permission mode without a restart.
	SetPermissionMode(mode string) error
	// SetModel switches the model for subsequent turns.
	SetModel(model string) error
	// Pipe forwards subprocess output into the program. Run it in a goroutine.
	Pipe(p *tea.Program)
	// Close ends the session. See the ordering constraint above.
	Close()
}

// Compile-time proof that the claude backend satisfies the seam. main already
// forces this by passing one to newModel, but stating it here keeps the check
// attached to the interface rather than to whichever call site happens to
// exist.
var _ Engine = (*claudeEngine)(nil)

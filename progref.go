// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// progRef is a shared handle on the running Bubble Tea program.
//
// It exists because of an ordering problem with no neat answer in Bubble Tea:
// the program is constructed *from* the model, so the model cannot be given the
// program at construction, and every later copy of the model is a value copy.
// A pointer to this holder survives those copies, and main fills it in once the
// program exists.
//
// One consumer: switching backend at runtime has to start piping a new engine
// into the program that is already running (backendswitch.go). Nothing else
// needs the program, and nothing else should — a model that can reach the
// program can bypass the update loop, which is how ordering bugs start.
//
// The same trick as model.content, which is a *strings.Builder for the same
// value-copy reason.
type progRef struct {
	mu sync.Mutex
	p  *tea.Program
}

// set is called once by main, after the program is constructed.
func (r *progRef) set(p *tea.Program) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.p = p
}

// get returns the program, or nil before main has set it.
func (r *progRef) get() *tea.Program {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.p
}

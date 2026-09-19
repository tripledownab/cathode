// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"testing"
)

// TestMain points the whole package at a throwaway state dir.
//
// Any test that builds a model opens the session store and the prompt history,
// and most of them never redirected $XDG_STATE_HOME — so `go test` read the
// developer's own state, and because an established prompt history sits at its
// cap, the cap self-heal rewrote their real prompt-history.jsonl.
//
// Doing it here rather than in each test is what makes that unreachable: a new
// test cannot touch those files by forgetting a t.Setenv. A test that needs a
// specific dir still overrides this with its own t.Setenv, which is scoped to
// that test and restored after it.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "cathode-test-state-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot create a test state dir:", err)
		os.Exit(1)
	}
	if err := os.Setenv("XDG_STATE_HOME", dir); err != nil {
		fmt.Fprintln(os.Stderr, "cannot redirect XDG_STATE_HOME:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir) // not deferred: os.Exit does not run defers
	os.Exit(code)
}

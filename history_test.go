// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"path/filepath"
	"testing"
)

// newTestHistory returns a history backed by a tempdir-scoped file so tests
// don't touch the user's real prompt-history.jsonl.
func newTestHistory(t *testing.T) *history {
	t.Helper()
	return &history{path: filepath.Join(t.TempDir(), "prompt-history.jsonl")}
}

func TestHistoryAppendDedupesAdjacent(t *testing.T) {
	h := newTestHistory(t)
	h.Append("hello")
	h.Append("hello") // adjacent dup — should be ignored
	h.Append("world")
	h.Append("hello") // not adjacent — keep
	if got := len(h.entries); got != 3 {
		t.Fatalf("entries = %d, want 3 (got %#v)", got, h.entries)
	}
}

func TestHistoryAppendCapsAtMax(t *testing.T) {
	h := newTestHistory(t)
	for i := 0; i < maxHistoryEntries+10; i++ {
		h.Append(string(rune('a' + i%26)))
	}
	if got := len(h.entries); got != maxHistoryEntries {
		t.Fatalf("entries = %d, want %d", got, maxHistoryEntries)
	}
}

// The prompt-history file is shared by every cathode instance, and the cap is
// small enough that an established history reaches the cap self-heal on nearly
// every turn. So load must re-read the file rather than rebuild it from one
// instance's memory, or each window erases the other's prompts as fast as they
// are typed.
func TestHistoryKeepsWhatAnotherInstanceAppended(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt-history.jsonl")
	a, b := &history{path: path}, &history{path: path}

	// Fill past the cap, so every further append reaches the self-heal.
	for i := 0; i < maxHistoryEntries; i++ {
		a.Append(fmt.Sprintf("a-%02d", i))
	}
	b.load() // B starts up and sees A's history
	a.Append("typed in window A")
	b.Append("typed in window B")

	fresh := &history{path: path}
	fresh.load()
	var seenA, seenB bool
	for _, e := range fresh.entries {
		switch e.Input {
		case "typed in window A":
			seenA = true
		case "typed in window B":
			seenB = true
		}
	}
	if !seenA || !seenB {
		t.Errorf("window A kept = %v, window B kept = %v; want both", seenA, seenB)
	}
	if len(fresh.entries) > maxHistoryEntries {
		t.Errorf("entries = %d, want the cap held at %d", len(fresh.entries), maxHistoryEntries)
	}
}

// TestHistoryMoveWalksAndReturnsToLive pins the cursor semantics: Ctrl-Up
// recalls the newest entry, again recalls the one before, Ctrl-Down walks back
// toward live and finally clears the input.
func TestHistoryMoveWalksAndReturnsToLive(t *testing.T) {
	h := newTestHistory(t)
	h.Append("one")
	h.Append("two")
	h.Append("three")

	// Ctrl-Up from live empty → newest.
	v, ok := h.Move(-1, "")
	if !ok || v != "three" {
		t.Fatalf("first Up: (%q,%v), want (three,true)", v, ok)
	}
	// Ctrl-Up while currentInput matches recalled → next-older.
	v, ok = h.Move(-1, "three")
	if !ok || v != "two" {
		t.Fatalf("second Up: (%q,%v), want (two,true)", v, ok)
	}
	// Ctrl-Down → newest again.
	v, ok = h.Move(1, "two")
	if !ok || v != "three" {
		t.Fatalf("Down: (%q,%v), want (three,true)", v, ok)
	}
	// Ctrl-Down → live (empty).
	v, ok = h.Move(1, "three")
	if !ok || v != "" {
		t.Fatalf("Down to live: (%q,%v), want (\"\",true)", v, ok)
	}
}

// TestHistoryMoveProtectsInFlightTyping pins the rule that lets you keep what
// you've typed: at live with text, Ctrl-Up is a no-op; on a recalled entry
// you've edited away from, the walk aborts.
func TestHistoryMoveProtectsInFlightTyping(t *testing.T) {
	h := newTestHistory(t)
	h.Append("recall me")

	// Live with text typed → don't blow it away.
	if _, ok := h.Move(-1, "draft I'm writing"); ok {
		t.Fatal("Ctrl-Up at live with typed text should be a no-op")
	}
	// Recall, then edit, then Ctrl-Up → abort.
	v, ok := h.Move(-1, "")
	if !ok || v != "recall me" {
		t.Fatalf("setup recall failed: (%q,%v)", v, ok)
	}
	if _, ok := h.Move(-1, "recall me + edits"); ok {
		t.Fatal("Ctrl-Up after editing recalled entry should be a no-op")
	}
}

// TestHistoryAppendResetsCursor pins that sending a new prompt while walked
// back returns the cursor to live, so the next Ctrl-Up starts fresh from the
// newest entry.
func TestHistoryAppendResetsCursor(t *testing.T) {
	h := newTestHistory(t)
	h.Append("a")
	h.Append("b")
	_, _ = h.Move(-1, "") // cursor at -1, recalled "b"
	h.Append("c")
	if h.cursor != 0 {
		t.Fatalf("cursor after append = %d, want 0", h.cursor)
	}
	v, ok := h.Move(-1, "")
	if !ok || v != "c" {
		t.Fatalf("after Append, Ctrl-Up should recall newest: (%q,%v)", v, ok)
	}
}

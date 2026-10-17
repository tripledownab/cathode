// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

// lockState does not lock on Windows, which has no flock. Two instances can then
// interleave a write, and the later one wins.
//
// That is a far smaller window than the bug the lock exists for: the write is a
// read-modify-write either way, so what is at risk is the microseconds between
// the read and the rename, not everything another instance recorded since this
// one started. Reaching for a lockfile instead would trade this for a stale lock
// that wedges every instance, which is worse.
func lockState(string) (func(), error) { return func() {}, nil }

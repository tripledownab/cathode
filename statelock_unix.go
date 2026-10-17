// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"os"
	"syscall"
)

// lockState takes an exclusive advisory lock and returns the release.
//
// flock and not a lockfile created with O_EXCL, because the kernel drops this
// one when the process dies. A cathode that is killed mid-write would otherwise
// leave a lockfile behind that every other instance waits on forever, and
// breaking a stale lock needs a timeout nobody can pick correctly.
//
// The lock is its own file rather than the store, because the store is replaced
// by rename on every write: a lock on that inode would guard a file nobody is
// looking at any more. The lockfile itself is never replaced, so it stays in the
// state dir between runs.
//
// The wait is unbounded, which is right when every holder keeps it for one small
// read and write. If a cathode is ever seen frozen on a keypress, a sibling
// stopped mid-write is the thing to look for.
func lockState(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

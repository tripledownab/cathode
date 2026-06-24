package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// debug is the package-level sink for the -debug flag, set once by main.go
// before any goroutine touches it. nil = disabled, and `debug.Logf(...)` is a
// no-op on a nil receiver so the callsites in engine.go / approvals.go stay
// terse without explicit guards.
var debug *DebugLog

// DebugLog is a goroutine-safe tagged-line logger used by the -debug flag to
// capture raw stream-json (stdin/stdout) and MCP traffic to a file. A nil
// receiver is a no-op, so callsites stay terse (`d.Logf(...)` without a guard).
type DebugLog struct {
	mu sync.Mutex
	w  io.WriteCloser
}

// OpenDebugLog opens (or creates and appends to) path and writes a session
// header. Use Close on shutdown.
func OpenDebugLog(path string) (*DebugLog, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(f, "\n--- cathode debug log opened %s ---\n", time.Now().Format(time.RFC3339))
	return &DebugLog{w: f}, nil
}

// Logf writes one tagged line: "HH:MM:SS.mmm [tag] <formatted>".
func (d *DebugLog) Logf(tag, format string, args ...any) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	fmt.Fprintf(d.w, "%s [%s] ", time.Now().Format("15:04:05.000"), tag)
	fmt.Fprintf(d.w, format, args...)
	fmt.Fprintln(d.w)
}

// Close flushes and releases the underlying file.
func (d *DebugLog) Close() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_ = d.w.Close()
}

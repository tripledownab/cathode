// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeAppServer writes a stand-in for `codex app-server` and points
// codexCommand at it. The script speaks the real frame shapes, taken from a
// recorded session: a reply carries an id and no method, a notification carries
// a method and no id.
//
// This exercises the parts a unit test on the codec cannot reach — the reader
// goroutine, id correlation across concurrent calls, and the handshake's
// result.thread.id nesting — without a billed turn or a network call.
func fakeAppServer(t *testing.T, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-codex")
	script := "#!/bin/sh\n" + body
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := codexCommand
	t.Cleanup(func() { codexCommand = orig })
	codexCommand = path
}

// readLoop reads one request per line and answers from a case statement. Using
// the method name rather than the id keeps the script independent of how many
// calls the engine makes.
const echoServer = `
while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
  case "$line" in
    *'"initialize"'*)   printf '{"id":%s,"result":{"userAgent":"fake"}}\n' "$id" ;;
    *'"thread/start"'*) printf '{"method":"thread/started","params":{"thread":{"id":"th-1"}}}\n'
                        printf '{"id":%s,"result":{"thread":{"id":"th-1"},"model":"fake-model"}}\n' "$id" ;;
    *'"turn/start"'*)   printf '{"id":%s,"result":{"turn":{"id":"tu-1"}}}\n' "$id" ;;
  esac
done
`

func TestCodexInitializeOpensAThread(t *testing.T) {
	fakeAppServer(t, echoServer)

	e, err := newCodexEngine(codexEngineConfig{Mode: "ask", Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	if err := e.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	e.mu.Lock()
	got := e.threadID
	e.mu.Unlock()
	if got != "th-1" {
		t.Errorf("threadID = %q, want th-1 read from result.thread.id", got)
	}
}

// A turn cannot open before a thread exists, and saying so beats a wire error.
func TestCodexSendRefusesBeforeInitialize(t *testing.T) {
	fakeAppServer(t, echoServer)

	e, err := newCodexEngine(codexEngineConfig{Mode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	if err := e.Send("hello"); err == nil {
		t.Error("Send with no thread should report why, not reach the wire")
	}
}

// The turn id comes back on the turn/start reply, and Interrupt needs it
// alongside the thread id.
func TestCodexSendRecordsTheTurnID(t *testing.T) {
	fakeAppServer(t, echoServer)

	e, err := newCodexEngine(codexEngineConfig{Mode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := e.Send("hello"); err != nil {
		t.Fatal(err)
	}

	// Send is deliberately non-blocking, so the id lands on the reply goroutine.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		e.mu.Lock()
		got := e.turnID
		e.mu.Unlock()
		if got == "tu-1" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("turn id was never recorded from the turn/start reply")
}

// A backend that dies must not leave the UI waiting out a 60s timeout per call.
func TestCodexCallFailsWhenTheSubprocessExits(t *testing.T) {
	fakeAppServer(t, "exit 0\n")

	e, err := newCodexEngine(codexEngineConfig{Mode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	done := make(chan error, 1)
	go func() { _, err := e.call("initialize", map[string]any{}); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a call to a dead subprocess should fail, not succeed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("call hung after the subprocess exited; failAll did not fire")
	}
}

// Mode and model are per-turn on codex, so the setters record rather than send.
// They must not fail, and the next turn must carry the new value.
func TestCodexSettersRecordForTheNextTurn(t *testing.T) {
	fakeAppServer(t, echoServer)

	e, err := newCodexEngine(codexEngineConfig{Mode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	if err := e.SetPermissionMode("plan"); err != nil {
		t.Errorf("SetPermissionMode: %v", err)
	}
	if err := e.SetModel("gpt-x"); err != nil {
		t.Errorf("SetModel: %v", err)
	}
	e.mu.Lock()
	mode, model := e.mode, e.model
	e.mu.Unlock()
	if mode != "plan" {
		t.Errorf("mode = %q, want the cathode mode stored verbatim", mode)
	}
	if model != "gpt-x" {
		t.Errorf("model = %q", model)
	}
}

// The adapter turns frames into entries. thread/started names the session, and
// a token update sets the gauge from the window codex states outright.
func TestCodexAdapterMapsFramesToEntries(t *testing.T) {
	m, _ := newTestModel(t, "")

	m.handleCodexEvent(codexFrame{
		Method: "thread/started",
		Params: json.RawMessage(`{"thread":{"id":"th-abc","model":"gpt-test"}}`),
	})
	if m.session != "th-abc" {
		t.Errorf("session = %q, want the thread id", m.session)
	}
	// The model rides on the thread object; claude announces it separately.
	if m.modelID != "gpt-test" {
		t.Errorf("modelID = %q, want the model named on the thread", m.modelID)
	}

	m.handleCodexEvent(codexFrame{
		Method: "thread/tokenUsage/updated",
		Params: json.RawMessage(`{"tokenUsage":{"total":{"inputTokens":1000,"outputTokens":50},"modelContextWindow":258400}}`),
	})
	if m.ctxLimit != 258400 {
		t.Errorf("ctxLimit = %d, want the reported window rather than a guess", m.ctxLimit)
	}
	if m.ctxTokens != 1000 {
		t.Errorf("ctxTokens = %d", m.ctxTokens)
	}

	m.handleCodexEvent(codexFrame{
		Method: "item/completed",
		Params: json.RawMessage(`{"item":{"type":"agentMessage","id":"m1","text":"hello there"}}`),
	})
	last := m.entries[len(m.entries)-1]
	if last.kind != entAgent || last.text != "hello there" {
		t.Errorf("agent message = %+v, want entAgent", last)
	}

	// The user's own turn is already in the transcript; echoing it would double it.
	before := len(m.entries)
	m.handleCodexEvent(codexFrame{
		Method: "item/completed",
		Params: json.RawMessage(`{"item":{"type":"userMessage","id":"u1"}}`),
	})
	if len(m.entries) != before {
		t.Error("the echoed user message must not be added again")
	}
}

// The three file-change kinds, each carrying a different thing in `diff`. All
// three shapes were taken from the live CLI; getting the kind wrong renders a
// deletion as an addition, or a hunk as a wall of new text.
func TestCodexFileDiffsReadEachKindCorrectly(t *testing.T) {
	const hunk = "@@ -1,3 +1,3 @@\n line one\n-line two\n+line TWO\n line three\n"
	raw := json.RawMessage(`{"changes":[
		{"path":"/tmp/a.txt","kind":{"type":"add"},"diff":"hello\n"},
		{"path":"/tmp/b.txt","kind":{"type":"delete"},"diff":"gone\n"},
		{"path":"/tmp/c.txt","kind":{"type":"update"},"diff":` + mustJSON(hunk) + `},
		{"path":"/tmp/d.txt","kind":{"type":"martian"},"diff":"?"}
	]}`)

	ds := codexFileDiffs(raw, "/tmp")
	if len(ds) != 3 {
		t.Fatalf("got %d diffs, want 3 (the unknown kind is skipped)", len(ds))
	}
	if ds[0].file != "a.txt" {
		t.Errorf("path = %q, want it trimmed against the session root", ds[0].file)
	}
	if ds[0].new != "hello\n" || ds[0].old != "" {
		t.Errorf("add: got old=%q new=%q, want the content as the new side", ds[0].old, ds[0].new)
	}
	if ds[1].old != "gone\n" || ds[1].new != "" {
		t.Errorf("delete: got old=%q new=%q, want the content as the old side", ds[1].old, ds[1].new)
	}
	if ds[2].unified != hunk {
		t.Errorf("update: got unified=%q, want the supplied hunk verbatim", ds[2].unified)
	}
}

// A supplied hunk must reach the renderer untouched, and a computed one must
// still be computed. This is the pairing the refactor exists to make safe.
func TestUnifiedTextPrefersASuppliedDiff(t *testing.T) {
	const hunk = "@@ -1,1 +1,1 @@\n-a\n+b\n"
	if got := (fileDiff{file: "f", unified: hunk}).unifiedText(); got != hunk {
		t.Errorf("supplied diff was not used verbatim: %q", got)
	}
	computed := (fileDiff{file: "f", old: "a\n", new: "b\n"}).unifiedText()
	if computed == "" || !strings.Contains(computed, "+b") {
		t.Errorf("a before/after pair should still be diffed, got %q", computed)
	}
}

// A codex update renders as a real diff card, not a raw tool card. Without the
// unified path this fell through to addTool and showed the hunk as JSON.
func TestCodexUpdateRendersAsADiffCard(t *testing.T) {
	m, _ := newTestModel(t, "")
	m.handleCodexEvent(codexFrame{
		Method: "item/started",
		Params: json.RawMessage(`{"item":{"type":"fileChange","id":"fc1","changes":[
			{"path":"/tmp/x.go","kind":{"type":"update"},"diff":"@@ -1,1 +1,1 @@\n-a\n+b\n"}
		]}}`),
	})
	last := m.entries[len(m.entries)-1]
	if last.kind != entDiff {
		t.Fatalf("entry kind = %v, want entDiff", last.kind)
	}
	if len(last.diffs) != 1 || last.diffs[0].unified == "" {
		t.Errorf("diff entry = %+v, want the supplied hunk carried through", last.diffs)
	}
	out := stripANSI(renderDiffFor(diffUnified, last.diffs[0], 80))
	if !strings.Contains(out, "+ b") || !strings.Contains(out, "- a") {
		t.Errorf("the card should show the change, got:\n%s", out)
	}
}

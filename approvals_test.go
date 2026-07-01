package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func rpc(t *testing.T, url, body string) map[string]any {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestApprovalsHandshakeAndDecision(t *testing.T) {
	a, err := StartApprovals()
	if err != nil {
		t.Fatal(err)
	}

	// initialize
	init := rpc(t, a.url, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if init["result"] == nil {
		t.Fatalf("initialize had no result: %v", init)
	}

	// tools/list must advertise mcp tool "approve"
	list := rpc(t, a.url, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if !strings.Contains(mustJSON(list), "approve") {
		t.Fatalf("approve tool not listed: %s", mustJSON(list))
	}

	// tools/call -> a goroutine plays the TUI and allows it
	go func() {
		req := <-a.pending
		if req.toolName != "Edit" {
			t.Errorf("unexpected tool: %s", req.toolName)
		}
		req.reply <- approvalReply{allow: true}
	}()

	done := make(chan map[string]any, 1)
	go func() {
		done <- rpc(t, a.url, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"approve","arguments":{"tool_name":"Edit","input":{"file_path":"x.go"}}}}`)
	}()

	select {
	case call := <-done:
		s := mustJSON(call)
		if !strings.Contains(s, `\"behavior\":\"allow\"`) || !strings.Contains(s, "x.go") {
			t.Fatalf("expected allow with echoed input, got: %s", s)
		}
		t.Log("approvals allow path OK")
	case <-time.After(2 * time.Second):
		t.Fatal("tools/call timed out")
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestApprovalsSSEFraming(t *testing.T) {
	a, err := StartApprovals()
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("POST", a.url,
		strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected SSE content-type, got %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "event: message") || !strings.Contains(s, "data: ") {
		t.Fatalf("bad SSE framing: %q", s)
	}
	// the data payload must be valid JSON-RPC advertising the approve tool
	line := strings.SplitN(s[strings.Index(s, "data: ")+len("data: "):], "\n", 2)[0]
	var out map[string]any
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		t.Fatalf("SSE data not valid JSON: %v / %s", err, line)
	}
	if !strings.Contains(line, "approve") {
		t.Fatalf("approve tool missing from SSE payload: %s", line)
	}
	t.Log("SSE framing OK")
}

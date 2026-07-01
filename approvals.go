package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// Approvals is a minimal in-process MCP server (Streamable HTTP transport)
// exposing a single `approve` tool. Claude Code calls it via
// --permission-prompt-tool whenever a tool needs a permission decision that no
// static allow/deny rule already settled. Requests are funneled to the TUI over
// `pending`; the TUI sends back allow/deny on each request's reply channel.
//
// We hand-roll the protocol (rather than pull in an MCP SDK) because the surface
// is one tool and staying dependency-light keeps the binary small. This
// implements just enough of the spec for the request/response permission flow:
// initialize, tools/list, tools/call. If a future Claude Code client is stricter
// about the Streamable-HTTP handshake (SSE, session ids), this is the file to
// extend.
type Approvals struct {
	pending chan approvalReq
	url     string
	srv     *http.Server
}

// approvalReq is one pending permission decision surfaced to the TUI.
type approvalReq struct {
	toolName string
	input    json.RawMessage
	reply    chan approvalReply
}

// approvalReply is the TUI's answer to an approvalReq. allow runs the tool;
// otherwise it's denied and message becomes the denial text Claude sees — which
// for the AskUserQuestion tool we repurpose to carry the user's chosen answer
// (the only channel the headless CLI gives us to feed a question's result back).
type approvalReply struct {
	allow   bool
	message string
}

const (
	approvalsServer = "approvals"
	approvalsTool   = "approve"
)

// StartApprovals binds a localhost port and serves the MCP endpoint.
func StartApprovals() (*Approvals, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	a := &Approvals{
		pending: make(chan approvalReq),
		url:     fmt.Sprintf("http://%s/mcp", ln.Addr().String()),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", a.handle)
	a.srv = &http.Server{Handler: mux}
	go func() { _ = a.srv.Serve(ln) }()
	return a, nil
}

// permissionToolName is the value passed to --permission-prompt-tool.
func (a *Approvals) permissionToolName() string {
	return "mcp__" + approvalsServer + "__" + approvalsTool
}

// mcpConfigJSON is the inline --mcp-config that points Claude Code at us.
func (a *Approvals) mcpConfigJSON() string {
	return fmt.Sprintf(`{"mcpServers":{"%s":{"type":"http","url":"%s"}}}`,
		approvalsServer, a.url)
}

// ---- JSON-RPC plumbing ----

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (a *Approvals) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// Read the body up-front so -debug can tee the exact JSON-RPC payload the
	// client sent before we decode and dispatch.
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	debug.Logf("mcp-in", "%s", raw)
	var req rpcReq
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Notifications carry no id and expect no response body.
	if len(req.ID) == 0 || string(req.ID) == "null" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	resp := rpcResp{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		ver := "2025-06-18"
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(req.Params, &p) == nil && p.ProtocolVersion != "" {
			ver = p.ProtocolVersion // echo the client's version
		}
		w.Header().Set("Mcp-Session-Id", "ccharness")
		resp.Result = map[string]any{
			"protocolVersion": ver,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": approvalsServer, "version": "0.1.0"},
		}
	case "tools/list":
		resp.Result = map[string]any{"tools": []any{approveSchema()}}
	case "tools/call":
		resp.Result = a.callTool(req.Params)
	default:
		resp.Error = &rpcErr{Code: -32601, Message: "method not found: " + req.Method}
	}

	// Streamable HTTP lets us answer a POST with either a single JSON response
	// or an SSE stream. Honour the client's Accept header: if it advertises
	// text/event-stream, frame the reply as SSE (covers strict clients);
	// otherwise send plain JSON.
	if acceptsSSE(r.Header.Get("Accept")) {
		writeSSE(w, resp)
		return
	}
	writeJSON(w, resp)
}

// acceptsSSE reports whether the client's Accept header allows an SSE stream.
func acceptsSSE(accept string) bool {
	return strings.Contains(strings.ToLower(accept), "text/event-stream")
}

func writeJSON(w http.ResponseWriter, resp rpcResp) {
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(resp)
	debug.Logf("mcp-out", "%s", b)
	_, _ = w.Write(append(b, '\n'))
}

// writeSSE emits the response as a single SSE `message` event and flushes. For a
// request/response exchange we deliver the one response and let the handler
// return, which closes the stream.
func writeSSE(w http.ResponseWriter, resp rpcResp) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	b, _ := json.Marshal(resp)
	debug.Logf("mcp-out-sse", "%s", b)
	fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func approveSchema() map[string]any {
	return map[string]any{
		"name":        approvalsTool,
		"description": "Approve or deny a tool invocation requested by the agent.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tool_name": map[string]any{"type": "string"},
				"input":     map[string]any{"type": "object"},
			},
			"required": []string{"tool_name"},
		},
	}
}

// callTool blocks until the TUI decides, then returns the allow/deny JSON that
// Claude Code expects in the tool result text.
func (a *Approvals) callTool(params json.RawMessage) map[string]any {
	var p struct {
		Name      string `json:"name"`
		Arguments struct {
			ToolName string          `json:"tool_name"`
			Input    json.RawMessage `json:"input"`
		} `json:"arguments"`
	}
	_ = json.Unmarshal(params, &p)

	reply := make(chan approvalReply, 1)
	a.pending <- approvalReq{toolName: p.Arguments.ToolName, input: p.Arguments.Input, reply: reply}
	d := <-reply

	var decision string
	if d.allow {
		inp := string(p.Arguments.Input)
		if inp == "" {
			inp = "{}"
		}
		decision = fmt.Sprintf(`{"behavior":"allow","updatedInput":%s}`, inp)
	} else {
		msg := d.message
		if msg == "" {
			msg = "Denied by user in ccharness"
		}
		mb, _ := json.Marshal(msg)
		decision = fmt.Sprintf(`{"behavior":"deny","message":%s}`, mb)
	}
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": decision}},
	}
}

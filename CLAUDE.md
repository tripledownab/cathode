# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build / test

```bash
go mod tidy            # first-time: fetches deps and writes go.sum
make build             # -> ./cathode
make test              # go test ./...
go test -run TestApprovalsSSEFraming ./...   # single test
```

`go.sum` is intentionally absent from the repo — `go mod tidy` regenerates it locally against the canonical module proxy. The Go module is named `ccharness` and the repo directory is `cathode`, with the binary also `cathode` (set by `APP` in the Makefile) and the wordmark rendering as `cath0d3` (`appName` in `theme.go`); keep BUILD.md and README in sync when renaming. (The repo dir was renamed from `doorway`; because `claude` partitions its per-project session JSONLs by cwd slug under `~/.claude/projects/-<abs-path>`, a repo-path rename also requires moving that slug dir — else prior sessions stop surfacing in the resume picker.) The persisted state dir is `$XDG_STATE_HOME/cathode` (resolved in `state.go`); a one-time `migrateLegacyState` renames an old `$XDG_STATE_HOME/doorway` dir into it on first run so existing sessions/history/settings survive. All three stores (`sessions.go`, `history.go`, `settings.go`) go through `stateFilePath`/`stateDir` — don't re-derive the path inline, and change the dir name only with a matching migration.

Running the app requires the `claude` CLI on PATH with `claude login` already completed (Pro/Max account). Verify with `claude` + `/status` showing the subscription route — anything else means you'll bill the API.

## Architecture

Cathode is a Bubble Tea TUI that drives the `claude` CLI as a long-lived subprocess over its bidirectional stream-json protocol. The agent loop, context, tools, and auth all live inside `claude`; this program owns only the terminal UI and the stdin/stdout plumbing. That split is the reason the binary is small and the reason it can ride a Max subscription instead of an API key.

Process flow (`main.go` → `engine.go` → `ui.go`):

1. `main.go` parses flags, optionally starts the in-process approvals MCP server, then `NewEngine` spawns `claude -p --input-format stream-json --output-format stream-json --verbose ...`. `--verbose` is mandatory — without it print mode emits only the final result, not the event stream.
2. `Engine.Pipe` runs in a goroutine, scanning stdout NDJSON and forwarding each parsed `Envelope` (`events.go`) to the Bubble Tea program via `p.Send`. The Update loop dispatches on envelope `Type` (`system` / `assistant` / `result`) in `ui.go:handleEvent`.
3. `Engine.Send` writes one user turn per Enter as an `outUser` NDJSON envelope. This envelope shape is the under-documented half of the protocol — it matches the Agent SDK streaming-input format. If a future `claude` rejects it, check the Agent SDK streaming docs, not the stream-json output docs.

### Subscription billing — load-bearing constraint

`engine.go:scrubbedEnv` strips `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN` from the subprocess environment on purpose. Either one present makes `claude` silently bill the API instead of the subscription. Do not "fix" this by removing the scrub; if a code path needs to set credentials, treat that as a deliberate design change.

### Approvals (`approvals.go`)

The approval pane in `ask` mode works by exposing a hand-rolled in-process MCP server (Streamable HTTP transport) with one tool, `approve`. It's wired in by passing two extra flags to `claude`: `--mcp-config '<inline JSON pointing at our localhost server>'` and `--permission-prompt-tool mcp__approvals__approve`. Each gated tool call → JSON-RPC `tools/call` to us → we push an `approvalReq` onto a channel → the TUI shows a y/n bar → the decision is returned as the documented `{"behavior":"allow"|"deny", ...}` payload.

Key subtleties:
- The handler answers a POST as either plain JSON or an SSE `message` event, chosen from the request's `Accept` header. Some clients (including current `claude`) advertise `text/event-stream` and need the SSE framing. Both paths are unit-tested.
- The approval flow only fires for tools that no static allow/deny rule already settled. Do not pass `--allowedTools Edit,Write,MultiEdit` if you want those to surface in the approval pane.
- `bypass` mode skips starting the server entirely (nothing is gated, nothing to approve).
- If the spec surface needs to grow (GET SSE channel, `Mcp-Session-Id` round-tripping, etc.), `approvals.go:handle` is the single extension point.

### Diff rendering (`diff.go`)

`diffsForTool` recognises `Edit`, `Write`, and `MultiEdit` tool_use blocks and turns them into one-or-more `fileDiff{file, old, new}` pairs. `Write` reads the current file off disk for the "before" — this works because `claude` runs in the TUI's cwd. Anything else falls back to a plain tool card in `ui.go`. Diff cards are re-rendered on every `rebuild()` so they reflow on resize.

### UI rebuild model (`ui.go`)

The transcript is stored as a `[]entry` of raw text/data, not pre-rendered strings. `rebuild()` re-renders every entry into the viewport on each new entry and on resize, which is what lets markdown (Glamour) and diff cards reflow to a new width. If you add a new entry kind, add a case to `rebuild()` and remember the resize implications.

### Theme discipline (`theme.go`, `splash.go`)

The BBS look (leet/studly/ornament/scene-divider helpers) is applied to chrome only — banner, dividers, status, labels, splash. Claude's replies and the diff body stay plain and readable. Don't sprinkle `leet()`/`studly()` into transcript content. Reskin by swapping the nine palette constants in `theme.go`; the wordmark is `appName` in `theme.go` (rendered `cath0d3`), and the splash shows a random pick from `logoVariants` in `logos.go` (regenerate a row with `figlet -f <font> -w 200 "cath0d3" | tr '\140' "'"`).

## Flags worth knowing

- `-mode ask|plan|build|bypass` → `claude --permission-mode default|plan|acceptEdits|bypassPermissions` (mapped in `main.go:modeToPermission`)
- `-mcp <path>` → passed through as a second `--mcp-config` alongside the approvals one; both flags compose
- `-model <name>` → empty string means "let the account default win"
- `-spinner bar|shade|block|arrow|scan` → animated throbber frames in the status bar

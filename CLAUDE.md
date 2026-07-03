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

Process flow (`main.go` → `engine.go` → the UI loop, which is split across `model.go` / `update.go` / `view.go` / `keys.go` / `stream.go` / `render.go`):

1. `main.go` parses flags, optionally starts the in-process approvals MCP server, then `NewEngine` spawns `claude -p --input-format stream-json --output-format stream-json --verbose ...`. `--verbose` is mandatory — without it print mode emits only the final result, not the event stream. After `p.Run()` returns, `main` closes the subprocess — never do that from the Update loop (`Engine.Close` blocks on `Wait()` and deadlocks against the `Pipe` goroutine; that was the one-ctrl+c freeze).
2. `Engine.Pipe` runs in a goroutine, scanning stdout NDJSON and forwarding each parsed `Envelope` (`events.go`) to the Bubble Tea program via `p.Send`. The Update loop dispatches on envelope `Type` (`system` / `assistant` / `user` / `result` / `control_response`) in `stream.go:handleEvent`.
3. `Engine.Send` writes one user turn per Enter as an `outUser` NDJSON envelope. This envelope shape is the under-documented half of the protocol — it matches the Agent SDK streaming-input format. If a future `claude` rejects it, check the Agent SDK streaming docs, not the stream-json output docs. Slash commands we don't own are forwarded to `claude` the same way (that's how skills and plugin commands run).

### Subscription billing — load-bearing constraint

`engine.go:scrubbedEnv` strips `ANTHROPIC_API_KEY` and `ANTHROPIC_AUTH_TOKEN` from the subprocess environment on purpose. Either one present makes `claude` silently bill the API instead of the subscription. Do not "fix" this by removing the scrub; if a code path needs to set credentials, treat that as a deliberate design change.

### Approvals (`approvals.go`)

The approval pane in `ask` mode works by exposing a hand-rolled in-process MCP server (Streamable HTTP transport) with one tool, `approve`. It's wired in by passing two extra flags to `claude`: `--mcp-config '<inline JSON pointing at our localhost server>'` and `--permission-prompt-tool mcp__approvals__approve`. Each gated tool call → JSON-RPC `tools/call` to us → we push an `approvalReq` onto a channel → the TUI shows a y/n bar → the decision is returned as the documented `{"behavior":"allow"|"deny", ...}` payload.

Key subtleties:
- The handler answers a POST as either plain JSON or an SSE `message` event, chosen from the request's `Accept` header. Some clients (including current `claude`) advertise `text/event-stream` and need the SSE framing. Both paths are unit-tested.
- The approval flow only fires for tools that no static allow/deny rule already settled. Do not pass `--allowedTools Edit,Write,MultiEdit` if you want those to surface in the approval pane.
- `bypass` mode skips starting the server entirely (nothing is gated, nothing to approve).
- `AskUserQuestion` also arrives through this approve tool (not a permission — a question). `question.go` intercepts it, shows the options as a picker, and returns the chosen answer as the *deny message* — the only channel the headless CLI gives us (the tool itself errors in `-p` mode). Claude reads that message as the answer. This is why the reply channel carries an `approvalReply{allow, message}` rather than a bare bool, and why the question is presented even in `build`/`bypass` (never auto-approved).
- If the spec surface needs to grow (GET SSE channel, `Mcp-Session-Id` round-tripping, etc.), `approvals.go:handle` is the single extension point.

### Diff rendering (`diff.go`, `diff_split.go`)

`diffsForTool` recognises `Edit`, `Write`, and `MultiEdit` tool_use blocks and turns them into one-or-more `fileDiff{file, old, new}` pairs. `Write` reads the current file off disk for the "before" — this works because `claude` runs in the TUI's cwd. Anything else falls back to a typed tool card (`tools.go`) or the plain card in `render.go`. `renderDiffFor` dispatches on the persisted diff style: unified (`diff.go`) or side-by-side split (`diff_split.go`, falls back to unified under 80 cols).

### UI rebuild model (`render.go`)

The transcript is stored as a `[]entry` of raw text/data, not pre-rendered strings. `rebuild()` renders each entry **once** into a retained buffer (per-entry cache keyed by wrap width) — the common case appends only the new tail, which is what keeps long sessions O(new) per message instead of the old O(n²). A full re-render happens only when the width changes or entries were removed; anything else that changes how existing entries render (theme swap, diff-style toggle) must go through `rerender()`, which drops the cache first. The composed frame body (viewport + scrollbar + sidebar) is additionally memoized per `bodyKey` (`view.go:refreshBody`), so typing and header animation don't re-style the transcript. If you add a new entry kind, add a case to `renderEntry()`; if you add a rendering-relevant setting, key `bodyKey` on it and route its commit through `rerender()`.

### Theme discipline (`theme.go`, `splash.go`)

The BBS look (leet/studly/ornament/scene-divider helpers) is applied to chrome only — banner, dividers, status, labels, splash. Claude's replies and the diff body stay plain and readable. Don't sprinkle `leet()`/`studly()` into transcript content. Theming is the `palettes` map in `theme.go` (11 built-in themes, ten colors each, switched live via `/theme` and persisted); add a theme by adding a palette row + a `themes` entry — every style rebuilds from the active palette in `buildStyles`. The wordmark is `appName` in `theme.go` (rendered `cath0d3`), and the splash shows a random pick from `logoVariants` in `logos.go` (regenerate a row with `figlet -f <font> -w 200 "cath0d3" | tr '\140' "'"`). The marketing SVGs and per-theme shots in `assets/` regenerate from live UI code via `CATHODE_GENASSETS=1 go test -run 'TestGenerateAssets|TestGenerateThemeAssets'` — regenerate them whenever chrome the preview shows (status bar, banner, diff card) changes.

## Flags worth knowing

- `-mode ask|plan|build|bypass` → `claude --permission-mode default|plan|acceptEdits|bypassPermissions` (mapped in `main.go:modeToPermission`)
- `-mcp <path>` → passed through as a second `--mcp-config` alongside the approvals one; both flags compose
- `-model <name>` → empty string means "let the account default win"
- `-spinner bar|shade|block|arrow|scan` → animated throbber frames in the status bar
- `-resume <session-id>` → resume a claude session; also set automatically when the user picks one via `ctrl+r` (main re-execs itself with it)
- `-ctx 200k|500k|1m|<n>` → the context-pressure gauge's window; auto-grows past the observed input
- `-debug <file>` → tee raw stream-json + MCP traffic to a logfile — the first thing to reach for on protocol issues

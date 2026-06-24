# Cathode

*A personal, BBS-styled terminal harness for Claude — running on your Max plan.*

> The binary is `cathode`, the Go module is `ccharness`, and the wordmark renders
> as `cath0d3` (`appName` in `theme.go`). The repo directory is still `doorway`.

## What it is

Cathode is a single-binary terminal UI that drives Claude Code. You type, Claude
works, and the conversation — replies, tool calls, file edits — streams into a
custom TUI with a 90s bulletin-board aesthetic. It is built for one user (you),
on a Mac and on Ubuntu, and it bills against your Claude Max subscription rather
than the pay-per-token API.

The name is a deliberate nod: on a BBS, a *door* was an external program the
board shelled out to — door games and the like. Cathode does exactly that with
the `claude` binary. The harness is the board; Claude is the door.

## The constraint that shaped everything

The whole design follows from one requirement: **use the Max subscription, not
API billing.** That sounds like a small detail, but it dictates the
architecture, because there are only two ways software talks to Claude:

The clean way is to drive the official `claude` binary as a subprocess. The
binary authenticates against whatever `claude login` established, so if no API
key is present in its environment, it draws from your plan. This is subscription
use of Claude Code itself — exactly what the plan is for.

The other way is to be a native agent that calls Anthropic's API directly. That
needs an API key. The only way such a tool reaches a subscription is by routing
a subscription OAuth token through the API — and as of early 2026 Anthropic
restricts that pattern to its own clients. Tools that relied on it (OpenCode,
Crush's old support) had it removed or blocked.

So Cathode drives the binary. The engine *is* Claude Code; we wrap it.

## Why not fork an existing TUI

The obvious move was to fork something polished — Crush or OpenCode, both
gorgeous, both MIT, both Bubble Tea. We didn't, for one structural reason: they
are native API-client agents. Their Claude-Max support is precisely the
restricted OAuth-routing pattern, which is why it was pulled. Forking either and
keeping Max clean would mean ripping out its engine and auth and replacing them
with a subprocess driver — which leaves only the UI.

So we inverted it. We kept our own clean subprocess engine and *borrowed their
view craft* (markdown rendering, diff cards, plan/build modes), all of which is
MIT-licensed and engine-agnostic. Best of both: their polish, our auth story.

## Architecture

Cathode splits cleanly into two layers that never share mutable state — they
hand off through channels.

The **engine** (`engine.go`) spawns `claude -p --input-format stream-json
--output-format stream-json --verbose` as a long-lived subprocess. We write user
turns to its stdin as NDJSON and read a stream of events back from its stdout.
The subprocess environment is scrubbed of `ANTHROPIC_API_KEY` and
`ANTHROPIC_AUTH_TOKEN` so nothing silently overrides the subscription.

The **UI** (`ui.go`) is a Bubble Tea program. A reader goroutine parses each
stdout event and forwards it into the update loop; the model turns events into
transcript entries and paints them. This is where the whole custom experience
lives — we own every pixel and none of the agent logic.

Permission handling is the interesting bit. In headless mode there is no
interactive prompt, so to approve actions inline we run a tiny **MCP server**
(`approvals.go`) inside the same process and point Claude at it with
`--permission-prompt-tool`. When Claude wants to use a gated tool, it calls our
`approve` tool over HTTP; the handler blocks, the TUI shows the proposed change
and a lightbar, your keypress flows back, and the handler returns allow or deny.
It speaks both transports the spec allows — plain JSON or an SSE stream — chosen
from the client's `Accept` header.

## The pieces

| file           | role                                                              |
|----------------|-------------------------------------------------------------------|
| `engine.go`    | subprocess lifecycle, env scrub, NDJSON in/out                    |
| `events.go`    | tolerant parser for the stream-json event envelope                |
| `ui.go`        | the Bubble Tea model — transcript, input, event handling          |
| `diff.go`      | edit-tool detection + the line-numbered red/green diff card       |
| `approvals.go` | in-process MCP permission server (JSON + SSE)                     |
| `theme.go`     | the 90s BBS / ANSI-art theme — palette, boxes, banner, status     |
| `main.go`      | flags, and the wiring that ties the three layers together         |

No heavy dependencies: Bubble Tea, Glamour, and `go-udiff` for the view; the
approvals server is pure standard library.

## What it does today

Claude's replies render as proper markdown through Glamour, so code blocks and
lists read correctly. File edits (`Edit`/`Write`/`MultiEdit`) become a diff
card — filename, change counts, an old/new line-number gutter, and red/green
hunks — re-rendered to width on resize. Three modes map to Claude's permission
posture: `plan` previews without touching files, `build` auto-accepts edits, and
`ask` routes every gated action through the inline approval pane. Your internal
tools attach as MCP servers via a `.mcp.json` and show up with no UI changes.

## The look

Authentic BBS: base-16 ANSI neon on black, CP437 double-line borders, `░▒▓█`
gradient flourishes on the wordmark, a magenta lightbar for approvals, and a
DOS-style status line reading `ALT-X EXIT │ MODE │ BAUD MAX │ NODE │ $cost`. The
entire look is nine palette constants at the top of `theme.go`; swap them for a
Turbo-Vision blue scheme or anything else without touching another file.

## Running it

```bash
claude login        # once, with Pro/Max credentials only
go mod tidy
go run . -mode ask
```

Before relying on it, run `claude` once interactively and confirm `/status`
shows the subscription route rather than API credits.

## Honest edges

The harness has not yet been driven by the real `claude` binary end to end — the
sandbox it was built in didn't have it installed. The most likely place for a
first-run hiccup is the MCP handshake in `approvals.go`; the JSON-RPC and both
transports are unit-tested, but a stricter real client might want more of the
Streamable-HTTP spec (a GET SSE channel, session-id round-tripping), and that
function is the one spot to extend.

Two protocol details are under-documented and worth knowing: the stdin user-turn
envelope matches the Agent SDK's streaming-input format, and the
permission-prompt-tool contract (`tool_name` + `input` in, `{"behavior":...}`
out) is the de-facto one. Both are flagged in the code.

## What's next

Live token-by-token streaming (it trades off against markdown, so it wants a
"plain while streaming, re-render on block-stop" pass); syntax-token
highlighting inside the diff (Glamour already pulls in a highlighter); and a
Glamour theme tuned to the BBS palette so Claude's markdown stops clashing with
the chrome. None are load-bearing — the core harness is complete.

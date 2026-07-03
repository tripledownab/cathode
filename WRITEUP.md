# Cathode

*A personal, BBS-styled terminal harness for Claude — running on your Max plan.*

> The binary is `cathode`, the Go module is `ccharness`, and the wordmark renders
> as `cath0d3` (`appName` in `theme.go`). The repo lives at
> `github.com/tripledownab/cathode` (it began life as `doorway`).

## What it is

Cathode is a single-binary terminal UI that drives Claude Code. You type, Claude
works, and the conversation — replies, tool calls, file edits — streams into a
custom TUI with a 90s bulletin-board aesthetic. It is built for one user (you),
on a Mac and on Ubuntu, and it bills against your Claude Max subscription rather
than the pay-per-token API.

The design carries a deliberate BBS nod: on a board, a *door* was an external
program the BBS shelled out to — door games and the like. This does exactly
that with the `claude` binary (the harness is the board; Claude is the door),
which is why the project was first called *Doorway*. *Cathode* keeps the same
era's glow — the CRT the whole aesthetic is drawn on.

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

The **UI** is a Bubble Tea program, split into small single-concern files
(`model.go`, `update.go`, `view.go`, `keys.go`, `stream.go`, `render.go`, …). A
reader goroutine parses each stdout event and forwards it into the update loop;
the model turns events into transcript entries and paints them. This is where
the whole custom experience lives — we own every pixel and none of the agent
logic.

Permission handling is the interesting bit. In headless mode there is no
interactive prompt, so to approve actions inline we run a tiny **MCP server**
(`approvals.go`) inside the same process and point Claude at it with
`--permission-prompt-tool`. When Claude wants to use a gated tool, it calls our
`approve` tool over HTTP; the handler blocks, the TUI shows the proposed change
and a lightbar, your keypress flows back, and the handler returns allow or deny.
It speaks both transports the spec allows — plain JSON or an SSE stream — chosen
from the client's `Accept` header.

## The pieces

The core pieces (the full per-file map lives in the README):

| file           | role                                                              |
|----------------|-------------------------------------------------------------------|
| `engine.go`    | subprocess lifecycle, env scrub, NDJSON in/out                    |
| `events.go`    | tolerant parser for the stream-json event envelope                |
| `stream.go`    | routes each parsed event into the model                           |
| `model.go` `update.go` `view.go` `keys.go` | the Bubble Tea loop — state, dispatch, paint, keys |
| `render.go`    | entries → viewport, with a per-entry render cache                 |
| `diff.go` `diff_split.go` | edit-tool detection + the unified / side-by-side diff cards |
| `approvals.go` | in-process MCP permission server (JSON + SSE)                     |
| `question.go`  | answers Claude's `AskUserQuestion` through a picker               |
| `theme.go`     | palettes (11 themes), boxes, banner, status                       |
| `main.go`      | flags, and the wiring that ties the layers together               |

No heavy dependencies: Bubble Tea, Glamour, and `go-udiff` for the view; the
approvals server is pure standard library.

## What it does today

Claude's replies render as proper markdown through Glamour, so code blocks and
lists read correctly; URLs come out as clickable OSC 8 hyperlinks, and extended
thinking shows dim above the reply. File edits (`Edit`/`Write`/`MultiEdit`)
become a diff card — filename, change counts, an old/new line-number gutter,
red/green hunks, unified or side-by-side — re-rendered to width on resize. Four
modes map to Claude's permission posture: `plan` previews without touching
files, `build` auto-accepts, `bypass` gates nothing, and `ask` routes every
gated action through the inline approval pane. When Claude *asks* something
(its `AskUserQuestion` tool), the options pop up as a picker and your choice
flows back — in every mode. Sessions resume via `ctrl+r`, the prompt takes
multi-line input, slash commands we don't own are forwarded to `claude` (so
skills and plugin commands work), and your internal tools attach as MCP servers
via a `.mcp.json` with no UI changes.

## The look

Authentic BBS: base-16 ANSI neon on black, CP437 double-line borders, `░▒▓█`
gradient flourishes on the wordmark, a magenta lightbar for approvals, and a
DOS-style status line reading `MDL │ MODE │ NODE │ BR │ CTX-gauge │ OUT │ $cost`
with a `READY`/`WORKING` state pinned right. The entire look is one palette row
(ten colors) in `theme.go` — eleven ship built in (Dracula, Nord, Catppuccin
Mocha, …), switched live with `/theme` and persisted, without touching another
file.

## Running it

```bash
claude login        # once, with Pro/Max credentials only
go mod tidy
go run . -mode ask
```

Before relying on it, run `claude` once interactively and confirm `/status`
shows the subscription route rather than API credits.

## Honest edges

The harness is in daily use against the real `claude` binary now; the original
worry — the MCP handshake in `approvals.go` — turned out to need exactly one
accommodation (the client advertises `text/event-stream`, so the handler speaks
SSE framing as well as plain JSON; both are unit-tested). If a future client
wants more of the Streamable-HTTP spec (a GET SSE channel, session-id
round-tripping), that handler is still the one spot to extend.

Two protocol details are under-documented and worth knowing: the stdin user-turn
envelope matches the Agent SDK's streaming-input format, and the
permission-prompt-tool contract (`tool_name` + `input` in, `{"behavior":...}`
out) is the de-facto one. Both are flagged in the code. A third rides on the
second: answering Claude's questions works by returning the user's choice as the
permission *deny message* — the only response channel the headless CLI exposes —
which Claude then reads as the answer.

## What's next

Live token-by-token streaming (it trades off against markdown, so it wants a
"plain while streaming, re-render on block-stop" pass); syntax-token
highlighting inside the diff (Glamour already pulls in a highlighter); a Glamour
theme tuned to the active palette so Claude's markdown stops clashing with the
chrome; and multi-select / free-text answers for Claude's questions
(single-select works today). None are load-bearing — the core harness is
complete.

package main

import (
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

// entryKind tags one item in the transcript so rebuild() can dispatch to the
// right renderer. We keep raw text/data per entry and re-render on resize so
// markdown re-wraps to the new width.
type entryKind int

const (
	entUser entryKind = iota
	entClaude
	entTool
	entToolResult
	entDiff
	entInfo
	entError
)

type entry struct {
	kind       entryKind
	text       string          // user/claude: raw; tool fallback: "name\ninput"; info/error: plain
	diffs      []fileDiff      // entDiff only; rendered at current width on rebuild
	toolName   string          // entTool / entToolResult: resolved tool name (may be "")
	toolInput  json.RawMessage // entTool only; raw tool_use input JSON
	toolResult string          // entToolResult only; flattened result body
	toolError  bool            // entToolResult only; true if the tool reported an error
}

// model is the Bubble Tea model. Field grouping mirrors the lifecycle:
// external services up top, modal flags, widgets, then session/turn state.
type model struct {
	engine    *Engine
	approvals *Approvals
	md        *glamour.TermRenderer
	hist      *history
	sessions  *sessionStore

	pending     *approvalReq // non-nil while awaiting a y/n decision
	picker      *picker      // non-nil while a picker dialog is open
	splash      bool         // true until the first keypress dismisses the boot screen
	splashFrame int          // current animation frame; clamps at splashFinalFrame
	logoIdx     int          // which splash wordmark variant this launch shows (picked once)
	colorPhase  int          // monotonic counter driving the header wordmark's rainbow sweep
	sidebar     bool         // true to render the BBS info rail (auto-hidden on narrow terms)
	help        bool         // true while the help modal is up; Esc dismisses
	mouse       bool         // mouse capture on (wheel scroll) vs off (terminal-native select/copy)

	settings    settings // persisted user config (see settings.go)
	headerStyle string   // live header animation id; previewed in /settings, committed to settings.Header

	vp    viewport.Model
	input textinput.Model
	sp    spinner.Model
	// follow pins the transcript to the latest line while Claude streams;
	// cleared when the user scrolls up to read back (see scroll.go).
	follow bool
	// animating/spinning track whether a header / spinner tick is already in
	// flight, so exactly one is armed and an idle screen (static header, not
	// busy) stops redrawing instead of waking the runtime many times a second.
	animating bool
	spinning  bool

	entries  []entry
	queue    []string          // user messages typed while busy; drained one per turn end
	toolUses map[string]string // tool_use_id -> tool name, so tool_result events can show what they're answering
	busy     bool
	mode     string
	session  string
	modelID  string
	models   []ModelChoice // model menu from the initialize handshake; drives /model (see models.go)
	lastCost float64
	// Running token totals across the session. ctxTokens is the most recent
	// turn's "live" context size (input + cache_read + cache_creation), which
	// is what drives the context-pressure gauge in the status bar. outTokens
	// is cumulative. ctxLimit defaults to 200K and auto-grows when observed
	// ctx exceeds it, so users on the 1M-context beta don't see a stuck ⚠.
	ctxTokens int
	outTokens int
	ctxLimit  int
	resumeID  string // picker selection; main.go reads after p.Run() and re-execs
	ready     bool
	w, h      int
}

func newModel(e *Engine, mode string, a *Approvals, spin, resumeID string) model {
	ti := textinput.New()
	ti.Placeholder = "Ask Claude…  (enter to send, esc to quit)"
	ti.Focus()
	ti.Prompt = "› "
	ti.CharLimit = 0

	sp := spinner.New()
	sp.Spinner = bbsSpinner(spin)

	// Seed a sensible default size so the splash (and the rest of the UI)
	// renders on the very first frame. If the initial tea.WindowSizeMsg
	// arrives late or never — which produced the original "starting…" hang on
	// first launch — we still paint, then reflow on the next resize.
	const defW, defH = 80, 24
	st := loadSettings()
	applyTheme(st.Theme) // re-skin all styles to the persisted theme before first paint
	m := model{
		engine: e, approvals: a,
		hist:     openHistory(),
		sessions: openSessionStore(),
		input:    ti, sp: sp,
		settings: st, headerStyle: st.Header,
		mode: mode, splash: true,
		// Start at frame 1 so the wordmark is visible on the first paint;
		// without this the user sees ~140ms of blank screen before the first
		// splash tick fires.
		splashFrame: 1,
		logoIdx:     pickLogoIdx(),
		w:           defW, h: defH,
		vp:     newTranscriptViewport(defW, defH-6),
		ready:  true,
		follow: true,
		mouse:  true, // started with tea.WithMouseCellMotion in main.go
	}
	m.input.Width = defW - 4
	m.makeRenderer()
	// On resume, replay the last N turns from claude's own JSONL so the
	// transcript isn't empty after re-exec. claude itself loads the session
	// into context — this is purely a visual rehydrate.
	if resumeID != "" {
		// Skip the boot splash: the user already picked the session, so drop
		// them straight back into the transcript.
		m.splash = false
		const replayMax = 40
		prior, ctxTok := loadPriorTranscript(resumeID, replayMax)
		if len(prior) > 0 {
			m.entries = append(m.entries, entry{kind: entInfo, text: fmt.Sprintf("— resumed · replaying last %d entries —", len(prior))})
			m.entries = append(m.entries, prior...)
		}
		// Seed the context gauge from the resumed session's last turn so it
		// reflects real usage immediately instead of 0% (limit grown in
		// main.go once the -ctx flag is applied).
		m.ctxTokens = ctxTok
	}
	return m
}

// observeCtx records the live context size and snaps ctxLimit up past it
// (200K → 500K → 1M → 2M …) so users on the long-context beta never see a
// stuck gauge. Shared by the per-turn usage path (stream.go) and the resume
// seed (main.go, after the -ctx flag sets the initial limit).
func (m *model) observeCtx(tokens int) {
	m.ctxTokens = tokens
	for m.ctxLimit > 0 && m.ctxTokens > m.ctxLimit {
		m.ctxLimit = nextCtxTier(m.ctxLimit)
	}
}

func (m model) Init() tea.Cmd {
	// The spinner and header ticks are armed lazily (only while busy / while the
	// header animates) so an idle screen stops redrawing — see armSpinnerIfNeeded
	// and armHeaderIfNeeded in update.go.
	cmds := []tea.Cmd{textinput.Blink, splashTick(), requestModels(m.engine)}
	if m.approvals != nil {
		cmds = append(cmds, waitApproval(m.approvals))
	}
	return tea.Batch(cmds...)
}

// makeRenderer builds a glamour renderer sized to the current width. Borrowed
// from how OpenCode/Crush render assistant output as styled markdown.
func (m *model) makeRenderer() {
	w := m.vp.Width - 2
	if w < 20 {
		w = 20
	}
	// Pin the style instead of WithAutoStyle(): auto probes the terminal's
	// background via OSC 11, and the reply races with Bubble Tea's raw-mode
	// stdin reader on first paint, leaking fragments like "b:2424/2727/3a3a\"
	// into the input field. The BBS palette is dark-on-black regardless.
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(w),
	)
	if err == nil {
		m.md = r
	}
}

// add appends a plain text entry (user/claude/info/error) and rebuilds.
func (m *model) add(k entryKind, text string) {
	m.entries = append(m.entries, entry{kind: k, text: text})
	m.rebuild()
}

func (m *model) addDiffs(ds []fileDiff) {
	m.entries = append(m.entries, entry{kind: entDiff, diffs: ds})
	m.rebuild()
}

func (m *model) addTool(name string, input json.RawMessage) {
	m.entries = append(m.entries, entry{kind: entTool, toolName: name, toolInput: input})
	m.rebuild()
}

func (m *model) addToolResult(name, body string, isErr bool) {
	m.entries = append(m.entries, entry{kind: entToolResult, toolName: name, toolResult: body, toolError: isErr})
	m.rebuild()
}

// pendingApprovalMsg delivers a permission request from the approvals server
// into the update loop.
type pendingApprovalMsg struct{ req approvalReq }

// waitApproval blocks on the next permission request. Re-issued after each
// decision so the next one is picked up.
func waitApproval(a *Approvals) tea.Cmd {
	return func() tea.Msg { return pendingApprovalMsg{req: <-a.pending} }
}

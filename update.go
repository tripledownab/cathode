package main

import (
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// Update is the Bubble Tea dispatcher. Most of the heavy lifting lives in
// per-msg helpers; this stays a small router. Keys go through handleKey
// (keys.go); stream envelopes through handleEvent (stream.go); the rest is
// inlined because it's one statement each.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.resizeViewport()
		m.input.Width = msg.Width - 4
		m.makeRenderer()
		m.rebuild()

	case tea.KeyMsg:
		nm, cmd, handled := m.handleKey(msg)
		if handled {
			return nm, cmd
		}
		m = nm

	case spinner.TickMsg:
		// Only advance / re-arm while busy; idle, we let it lapse so the status
		// spinner isn't repainting the whole screen for nothing.
		if m.busy {
			var cmd tea.Cmd
			m.sp, cmd = m.sp.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			m.spinning = false
		}

	case rainbowTickMsg:
		// Drift the header band and re-arm at the configured fps — but only while
		// it actually animates (style not "off", fps > 0, splash gone). Otherwise
		// stop, so an idle screen quits redrawing.
		if m.shouldAnimateHeader() {
			m.colorPhase++
			cmds = append(cmds, rainbowTick(m.settings.FPS))
		} else {
			m.animating = false
		}

	case splashTickMsg:
		// Once the splash is dismissed (or fully revealed) we stop re-arming
		// the tick so the runtime isn't woken up for no reason.
		if !m.splash || m.splashFrame >= splashFinalFrame {
			return m, nil
		}
		m.splashFrame++
		return m, splashTick()

	case streamMsg:
		m.handleEvent(msg.env)

	case pendingApprovalMsg:
		// Surface what's being approved either way (so AUTO mode still shows
		// a transcript record of what ran).
		if ds, ok := diffsForTool(msg.req.toolName, msg.req.input); ok {
			m.addDiffs(ds)
		} else {
			m.addTool(msg.req.toolName, msg.req.input)
		}
		// AUTO (build) means "go autonomously" — short-circuit the approval
		// instead of stalling on a y/n bar. acceptEdits alone only handles
		// file edits; this extends it to bash, grep, fetch, etc.
		if m.mode == "build" {
			msg.req.reply <- true
			m.add(entInfo, "✓ auto-approved "+msg.req.toolName)
			return m, waitApproval(m.approvals)
		}
		m.pending = &msg.req

	case streamClosedMsg:
		m.busy = false
		note := "session ended"
		if msg.err != nil {
			note = "stream closed: " + msg.err.Error()
		}
		m.add(entInfo, "— "+note+" —")
	}

	prevInput := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	m.vp, cmd = m.vp.Update(msg)
	cmds = append(cmds, cmd)
	m.syncScroll(msg, prevInput)
	// Arm the animation ticks if they should be running and aren't yet. Handlers
	// that return early (handled keys) arm them directly, so this only has to
	// catch the fall-through paths (stream events, resize, the ticks themselves).
	if c := m.armHeaderIfNeeded(); c != nil {
		cmds = append(cmds, c)
	}
	if c := m.armSpinnerIfNeeded(); c != nil {
		cmds = append(cmds, c)
	}
	return m, tea.Batch(cmds...)
}

// shouldAnimateHeader reports whether the header wordmark's color tick should be
// running: past the splash, an animated style, and a positive fps.
func (m *model) shouldAnimateHeader() bool {
	return !m.splash && m.headerStyle != headerOff && m.settings.FPS > 0
}

// armHeaderIfNeeded starts the header color tick when it should animate but none
// is in flight, returning the Cmd (or nil). Idempotent via m.animating.
func (m *model) armHeaderIfNeeded() tea.Cmd {
	if m.shouldAnimateHeader() && !m.animating {
		m.animating = true
		return rainbowTick(m.settings.FPS)
	}
	return nil
}

// armSpinnerIfNeeded starts the status spinner tick when busy and none is in
// flight, returning the Cmd (or nil). Idempotent via m.spinning.
func (m *model) armSpinnerIfNeeded() tea.Cmd {
	if m.busy && !m.spinning {
		m.spinning = true
		return m.sp.Tick
	}
	return nil
}

// flushQueue pops the front of the queue (if any), sends it, and re-enters
// busy mode. Called when a turn ends; remaining items wait for the next
// result event.
func (m *model) flushQueue() {
	if len(m.queue) == 0 {
		return
	}
	next := m.queue[0]
	m.queue = m.queue[1:]
	m.resizeViewport()
	m.add(entUser, next)
	if err := m.engine.Send(next); err != nil {
		m.add(entError, "send error: "+err.Error())
		return
	}
	if m.session != "" {
		cwd, _ := os.Getwd()
		m.sessions.Touch(m.session, m.modelID, cwd, truncFirst(next), time.Now())
	}
	m.busy = true
}

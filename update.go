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
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(msg)
		cmds = append(cmds, cmd)

	case rainbowTickMsg:
		// Drift the header wordmark's color band; re-arm so it keeps cycling.
		m.colorPhase++
		cmds = append(cmds, rainbowTick())

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
	return m, tea.Batch(cmds...)
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

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// handleKey is the keyboard dispatcher. Returns (newModel, cmd, handled). When
// handled=false the caller should fall through to the input/viewport piping
// so plain typing still reaches the textinput.
func (m model) handleKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	// The boot splash eats the first keypress. Dismissing it reveals the header,
	// so start its animation tick now (this path returns early, bypassing the
	// tick-arming at the tail of Update).
	if m.splash {
		m.splash = false
		return m, m.armHeaderIfNeeded(), true
	}
	// Help modal swallows everything except dismissal keys.
	if m.help {
		switch msg.String() {
		case "esc", "?", "q", "enter":
			m.help = false
		}
		return m, nil, true
	}
	// A picker (session resume, palette, file picker) is modal: route every
	// key through it until it returns a selection or cancels.
	if m.picker != nil {
		kind := m.picker.kind
		next, chosen := m.picker.Update(msg)
		m.picker = next
		// Live-preview pickers re-skin behind the dialog as the cursor moves;
		// Enter commits + persists, Esc reverts to the saved value.
		switch kind {
		case "header":
			switch {
			case chosen != "":
				m.commitHeaderStyle(chosen)
			case m.picker == nil:
				m.headerStyle = m.settings.Header
			default:
				if id := m.picker.focusedID(); id != "" {
					m.headerStyle = id
				}
			}
			// Switching to/from "off" toggles the animation; re-arm if it should
			// now run (no-op if a tick is already in flight).
			return m, m.armHeaderIfNeeded(), true
		case "fps":
			if chosen != "" {
				m.commitFPS(chosen)
			}
			return m, m.armHeaderIfNeeded(), true
		case "theme":
			switch {
			case chosen != "":
				m.commitTheme(chosen)
			case m.picker == nil:
				applyTheme(m.settings.Theme)
				m.rebuild()
			default:
				if id := m.picker.focusedID(); id != "" {
					applyTheme(id)
					m.rebuild()
				}
			}
			return m, nil, true
		}
		if chosen == "" {
			return m, nil, true
		}
		switch kind {
		case "sessions":
			m.resumeID = chosen
			return m, tea.Quit, true
		case "slash":
			nm, cmd, _ := runSlash(&m, "/"+chosen)
			return nm, cmd, true
		case "model":
			m.applyModel(chosen)
			return m, nil, true
		case "settings":
			// Top-level menu: open the chosen setting's picker, pre-positioned.
			switch chosen {
			case "header":
				p := newPicker("header", "HEADER ANIMATION", headerStyleItems(), m.w, m.h)
				p.setCursorTo(m.settings.Header)
				m.picker = p
			case "fps":
				p := newPicker("fps", "ANIMATION FPS", fpsItems(), m.w, m.h)
				p.setCursorTo(strconv.Itoa(m.settings.FPS))
				m.picker = p
			case "theme":
				p := newPicker("theme", "COLOR THEME", themeItems(), m.w, m.h)
				p.setCursorTo(m.settings.Theme)
				m.picker = p
			}
			return m, nil, true
		}
		return m, nil, true
	}
	// While a permission decision is pending, keys drive the approval, not
	// the text input. Default is ALLOW: Enter (or a stray letter that arrived
	// because you were mid-typing) consents. Deny requires the explicit Esc
	// gesture, so an accidental "n" while writing can't blow up an approval.
	if m.pending != nil {
		return m.handleApprovalKey(msg)
	}

	// Arrow keys: prompt history, except with mouse capture off (/mouse), where
	// the terminal turns the wheel into ↑/↓ — there we scroll the transcript so
	// older output can be brought into view to select. Ctrl-↑/↓ always do
	// history, so it stays reachable in selection mode.
	switch s := msg.String(); s {
	case "up", "down", "ctrl+up", "ctrl+down":
		down := s == "down" || s == "ctrl+down"
		if !m.mouse && (s == "up" || s == "down") {
			if down {
				m.vp.LineDown(3)
			} else {
				m.vp.LineUp(3)
			}
			m.follow = m.vp.AtBottom()
			return m, nil, true
		}
		delta := -1
		if down {
			delta = 1
		}
		if v, ok := m.hist.Move(delta, m.input.Value()); ok {
			m.input.SetValue(v)
			m.input.CursorEnd()
		}
		return m, nil, true
	}

	// PageUp/PageDown scroll the transcript. The viewport's own scroll keys
	// (space/f/b/u/d/j/k) are disabled because they collide with typing into the
	// prompt (see scroll.go), so paging is driven explicitly here. Scrolling up
	// pauses auto-follow; scrolling back to the bottom re-arms it.
	switch msg.String() {
	case "pgup":
		m.vp.ViewUp()
		m.follow = m.vp.AtBottom()
		return m, nil, true
	case "pgdown":
		m.vp.ViewDown()
		m.follow = m.vp.AtBottom()
		return m, nil, true
	}

	// Shift+Tab cycles permission mode without restarting the subprocess.
	// Order is increasing autonomy: PLAN → EDIT → AUTO → PLAN. bypass is
	// intentionally excluded — that mode disables approvals entirely.
	if msg.Type == tea.KeyShiftTab {
		if m.mode == "bypass" {
			m.add(entInfo, "bypass mode: restart with -mode to switch")
		} else {
			m.mode = nextMode(m.mode)
			if err := m.engine.SetPermissionMode(modeToPermission(m.mode)); err != nil {
				m.add(entError, "mode toggle failed: "+err.Error())
			} else {
				m.add(entInfo, "→ mode: "+modeLabel(m.mode))
			}
		}
		return m, nil, true
	}

	// Modal shortcuts.
	switch msg.String() {
	case "ctrl+r":
		cwd, _ := os.Getwd()
		m.picker = newPicker("sessions", "RESUME SESSION", sessionItems(m.sessions, cwd), m.w, m.h)
		return m, nil, true
	case "ctrl+t":
		m.picker = newPicker("slash", "COMMANDS", slashItems(), m.w, m.h)
		return m, nil, true
	case "ctrl+g":
		// Mirrors /sidebar. Ctrl-B is tmux's default prefix, so we don't bind it.
		nm, cmd, _ := runSlash(&m, "/sidebar")
		return nm, cmd, true
	case "?":
		// Only when input is empty so real "?" still types normally.
		if strings.TrimSpace(m.input.Value()) == "" {
			m.help = true
			return m, nil, true
		}
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		m.engine.Close()
		return m, tea.Quit, true
	case tea.KeyEsc:
		return m.handleEsc()
	case tea.KeyEnter:
		return m.handleEnter()
	}
	return m, nil, false
}

// handleApprovalKey routes a keypress while m.pending is non-nil. Defaults to
// allow; Esc denies; Ctrl-C exits.
func (m model) handleApprovalKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		m.pending.reply <- false
		m.add(entInfo, "✗ denied "+m.pending.toolName)
		m.pending = nil
		return m, waitApproval(m.approvals), true
	case "ctrl+c":
		m.pending.reply <- false
		m.engine.Close()
		return m, tea.Quit, true
	default:
		m.pending.reply <- true
		m.add(entInfo, "✓ approved "+m.pending.toolName)
		m.pending = nil
		return m, waitApproval(m.approvals), true
	}
}

// handleEsc cascades: clear queue → interrupt in-flight → quit. So a stray
// Esc with messages queued or work in flight never drops the whole session.
func (m model) handleEsc() (model, tea.Cmd, bool) {
	if len(m.queue) > 0 {
		dropped := len(m.queue)
		m.queue = nil
		m.resizeViewport()
		m.rebuild()
		m.add(entInfo, fmt.Sprintf("dropped %d queued message(s)", dropped))
		return m, nil, true
	}
	if m.busy {
		if err := m.engine.Interrupt(); err != nil {
			m.add(entError, "interrupt failed: "+err.Error())
		} else {
			m.add(entInfo, "✗ interrupted")
		}
		// Flip busy off proactively: claude may still emit late events, but
		// the user gets the prompt back immediately.
		m.busy = false
		return m, nil, true
	}
	m.engine.Close()
	return m, tea.Quit, true
}

// handleEnter dispatches a non-empty submission: slash commands run in-process,
// busy turns enqueue, otherwise we send to claude.
func (m model) handleEnter() (model, tea.Cmd, bool) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return m, nil, false
	}
	// Slash command intercept: "/foo [arg]" runs in-process instead of going
	// to the claude subprocess.
	if strings.HasPrefix(text, "/") {
		m.input.SetValue("")
		m.hist.Append(text)
		nm, cmd, _ := runSlash(&m, text)
		return nm, cmd, true
	}
	// Busy: queue the message instead of dropping it on the floor.
	if m.busy {
		m.queue = append(m.queue, text)
		m.hist.Append(text)
		m.input.SetValue("")
		m.resizeViewport()
		m.rebuild()
		return m, nil, true
	}
	m.add(entUser, text)
	if err := m.engine.Send(text); err != nil {
		m.add(entError, "send error: "+err.Error())
		return m, nil, true
	}
	m.hist.Append(text)
	// Capture the first prompt for the session so the resume picker has a
	// human-recognisable label.
	if m.session != "" {
		cwd, _ := os.Getwd()
		m.sessions.Touch(m.session, m.modelID, cwd, truncFirst(text), time.Now())
	}
	m.input.SetValue("")
	m.busy = true
	// Start the working spinner (this path returns early, so arm it here rather
	// than relying on the tail of Update).
	return m, m.armSpinnerIfNeeded(), true
}

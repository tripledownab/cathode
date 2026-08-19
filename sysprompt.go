package main

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ---- extra system prompt (`/sysprompt`) ----
//
// claude takes extra standing instructions as a *launch* flag
// (--append-system-prompt-file), appended to its default system prompt. There
// is no runtime equivalent: the engine's intent switch accepts only turn,
// interrupt, set_model, set_permission_mode, set_max_thinking_tokens,
// apply_flag_settings and seed_read_state — none of which touch the prompt. So
// toggling it has to restart the subprocess.
//
// Rather than build a second restart path, this reuses the one the session
// picker already uses (main.go:restartResuming): persist the setting, hand main
// the live session id, quit. The fresh process reads the setting, adds the
// flag, and resumes the same session, so the new instructions apply from the
// next turn on.
//
// claude's context survives in full — it reloads the session — but the visible
// transcript is replayed from the session log and capped at replayMax entries,
// so a long scrollback comes back shortened. That's the cost of the toggle.
//
// The text lives in its own file rather than settings.json because it's prose:
// multi-line, edited by hand, and awkward as a JSON string. Only the on/off bit
// is persisted with the other settings.

const sysPromptFile = "system-prompt.md"

// sysPromptPath is the prompt file inside the state dir. Goes through
// stateFilePath like every other store (see CLAUDE.md); "" if unresolvable.
func sysPromptPath() string {
	p, err := stateFilePath(sysPromptFile)
	if err != nil {
		return ""
	}
	return p
}

// loadSysPrompt reads the prompt text, seeding the file from defaultSysPrompt
// if it doesn't exist yet. Returns "" when there's nothing to append — a
// missing state dir, an unreadable file, or a file that's all whitespace.
func loadSysPrompt() string {
	p := sysPromptPath()
	if p == "" {
		return ""
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		if defaultSysPrompt == "" {
			return ""
		}
		_ = os.WriteFile(p, []byte(defaultSysPrompt), 0o644)
		return defaultSysPrompt
	}
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// sysPromptSummary describes the current prompt for the picker: its first line
// and size, or a pointer to where to write one.
func sysPromptSummary() string {
	text := loadSysPrompt()
	if text == "" {
		if p := sysPromptPath(); p != "" {
			return "no prompt yet — write one to " + p
		}
		return "no prompt yet (no state dir)"
	}
	first, _, multi := strings.Cut(text, "\n")
	first = strings.TrimSpace(first)
	// Runes, not bytes: a pasted prompt is prose, and slicing at a byte offset
	// cuts an em-dash or a curly quote in half and emits invalid UTF-8.
	if r := []rune(first); len(r) > 48 {
		first = string(r[:47]) + "…"
	}
	if multi {
		first += " …"
	}
	return first
}

// sysPromptArgs is the launch flag pair for the appended prompt, or nil when
// the toggle is off or there's nothing to append. Passing the file rather than
// the text keeps multi-line prose out of argv. Called once from main.
func sysPromptArgs(on bool) []string {
	if !on {
		return nil
	}
	p := sysPromptPath()
	if p == "" || loadSysPrompt() == "" {
		return nil
	}
	return []string{"--append-system-prompt-file", p}
}

// Toggle ids for the picker.
const (
	sysPromptOn  = "on"
	sysPromptOff = "off"
)

func sysPromptItems() []pickerItem {
	return []pickerItem{
		{id: sysPromptOn, title: "on", subtitle: "append it to claude's system prompt — " + sysPromptSummary()},
		{id: sysPromptOff, title: "off", subtitle: "run with claude's default system prompt only"},
	}
}

func sysPromptLabel(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// commitSysPrompt persists the toggle and restarts into it. Returns tea.Quit
// when a restart is actually going to happen — main re-execs from resumeID (see
// the file header). Callers must return the Cmd.
func (m *model) commitSysPrompt(id string) tea.Cmd {
	on := id == sysPromptOn
	if on == m.settings.SysPrompt {
		return nil // already there; don't restart for nothing
	}
	if on && loadSysPrompt() == "" {
		// Restarting to apply an empty prompt would be a no-op the user reads as
		// a bug. Say where to put the text instead.
		m.add(entError, "no system prompt to apply — write one to "+sysPromptPath()+" first")
		return nil
	}
	m.settings.SysPrompt = on
	saveSettings(m.settings)

	// The flag is read at startup, so it can't take effect until we restart.
	switch {
	case m.busy:
		m.add(entInfo, "→ extra system prompt: "+sysPromptLabel(on)+" · applies on restart (busy — not interrupting this turn)")
		return nil
	case m.session == "":
		// No session id yet (init hasn't landed), so there's nothing to resume
		// into; a restart here would drop the conversation.
		m.add(entInfo, "→ extra system prompt: "+sysPromptLabel(on)+" · applies on next launch")
		return nil
	}
	m.add(entInfo, "→ extra system prompt: "+sysPromptLabel(on)+" · restarting…")
	return m.restartResuming(m.session)
}

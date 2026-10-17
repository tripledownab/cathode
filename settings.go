// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ---- the persisted settings file ----

// settings is the persisted user config. Small and forward-compatible: unknown
// fields are ignored on load, missing ones take their default.
type settings struct {
	Header  string `json:"header"`
	Theme   string `json:"theme"`
	FPS     int    `json:"fps"`     // header animation frame rate; 0 → defaultFPS on load
	Diff    string `json:"diff"`    // diff card style: "unified" or "split"
	Sidebar string `json:"sidebar"` // info-rail side: "right" (default) or "left"
	Bar     string `json:"bar"`     // /compact progress animation (compactstyle.go)
	// Banner is "on" or "off": whether the wordmark banner and its session
	// divider are drawn. Set by the color theme (see themeDef.banner) and
	// overridable in /settings until the next theme switch.
	Banner string `json:"banner"`
	// SysPrompt toggles the user's own standing instructions onto claude's
	// system prompt, as an output style. The text lives in its own file, not
	// here — see sysprompt.go. Defaults to off: the extra prompt is opt-in.
	SysPrompt bool `json:"sysprompt"`
}

func defaultSettings() settings {
	return settings{Header: headerCyan, Theme: defaultTheme, FPS: defaultFPS, Diff: diffUnified, Sidebar: sidebarRight, Bar: barComet, Banner: bannerOn}
}

// settingsPath mirrors sessionsPath/historyPath: $XDG_STATE_HOME/cathode, else
// ~/.local/state/cathode. Returns "" if no state dir is resolvable (load/save
// then no-op, so the app still runs with defaults).
func settingsPath() string {
	p, err := stateFilePath("settings.json")
	if err != nil {
		return ""
	}
	return p
}

// loadSettings reads settings.json, falling back to defaults for any missing or
// unreadable field so a corrupt/partial file never blocks startup.
func loadSettings() settings {
	s := defaultSettings()
	p := settingsPath()
	if p == "" {
		return s
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, &s) // keep defaults on parse error
	if s.Header == "" {
		s.Header = headerCyan
	}
	if s.Theme == "" {
		s.Theme = defaultTheme
	}
	if s.FPS <= 0 {
		s.FPS = defaultFPS
	}
	if s.Diff == "" {
		s.Diff = diffUnified
	}
	if s.Sidebar == "" {
		s.Sidebar = sidebarRight
	}
	if s.Bar == "" {
		s.Bar = barComet
	}
	// Migrate the retired header:"hidden" value. It meant "no banner", which
	// is now settings.Banner. Without this, an old settings.json would come
	// back with an unknown animation id and a banner the user had hidden.
	if s.Header == headerHidden {
		s.Header = headerCyan
		s.Banner = bannerOff
	}
	if s.Banner == "" {
		s.Banner = bannerOn
	}
	return s
}

// saveSettings applies fn to the settings on disk and writes them back,
// best-effort; failures are silent (settings are a nicety, not load-bearing).
//
// It takes a mutator and not a settings value, and that is the whole point.
// Every cathode instance shares this file and each holds a copy loaded at its
// own start, so writing that copy back reverted every setting another window had
// changed since — a theme switch in one window undid a diff-style switch in
// another. A read-modify-write alone cannot fix it, because it cannot tell a
// stale field from a deliberate one. So the caller names the field it is
// changing, and the signature no longer accepts a whole stale struct.
func saveSettings(fn func(*settings)) {
	p := settingsPath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	if unlock, err := lockState(p + ".lock"); err == nil {
		defer unlock()
	}
	s := loadSettings() // re-read inside the lock, like every other state file
	fn(&s)
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		replaceFile(p, b)
	}
}

// commitSetting changes one setting in this model and on disk, so the two cannot
// disagree about what was changed. Every commit* function goes through it.
//
// The model's own copy is not re-read from the file on purpose: a setting another
// window changed must not alter this window's theme mid-session. The file merges;
// the running UI does not.
func (m *model) commitSetting(fn func(*settings)) {
	fn(&m.settings)
	saveSettings(fn)
}

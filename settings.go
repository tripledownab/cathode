// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// Header animation style ids. These are the values persisted in settings.json
// and the ids dispatched by renderHeader (rainbow.go).
const (
	headerCyan    = "cyan"
	headerTheme   = "theme"
	headerRainbow = "rainbow"
	headerPulse   = "pulse"
	headerAmber   = "amber"
	headerMagenta = "magenta"
	headerOff     = "off"

	// headerHidden is retired. It used to be a header *animation* value, which
	// conflated two different things: how the wordmark moves, and whether the
	// banner exists at all. That conflation is why a theme could not own
	// banner visibility without also discarding the user's animation choice.
	// Visibility now lives in settings.Banner; this const remains only so
	// loadSettings can migrate an old settings.json.
	headerHidden = "hidden"
)

// Banner visibility. Separate from the header animation because a theme owns
// whether the banner shows, while the user owns how its wordmark animates.
const (
	bannerOn  = "on"
	bannerOff = "off"
)

// bannerDef is one row in the /settings banner picker.
type bannerDef struct{ id, label, desc string }

var bannerModes = []bannerDef{
	{bannerOn, "shown", "the wordmark banner and its session divider"},
	{bannerOff, "hidden", "no banner or divider — gives the transcript 4 more rows"},
}

func bannerLabel(id string) string {
	for _, b := range bannerModes {
		if b.id == id {
			return b.label
		}
	}
	return id
}

func bannerItems() []pickerItem {
	items := make([]pickerItem, 0, len(bannerModes))
	for _, b := range bannerModes {
		items = append(items, pickerItem{id: b.id, title: b.label, subtitle: b.desc})
	}
	return items
}

// commitBanner shows or hides the banner and persists it. The banner occupies
// rows, so the viewport has to be resized whenever it appears or disappears.
func (m *model) commitBanner(id string) {
	if id == m.settings.Banner {
		return
	}
	m.settings.Banner = id
	saveSettings(m.settings)
	m.resizeViewport()
	m.add(entInfo, "→ banner: "+bannerLabel(id))
}

// headerStyleDef is one row in the /settings header picker.
type headerStyleDef struct{ id, label, desc string }

// headerStyles is the ordered set shown in the settings modal. Add a style by
// adding a case to renderHeader and a row here.
var headerStyles = []headerStyleDef{
	{headerTheme, "theme color", "shimmer in the active theme's primary color (matches the ornaments)"},
	{headerCyan, "cyan shimmer", "single bright-cyan brightness wave (fixed, ignores theme)"},
	{headerRainbow, "rainbow", "the full-spectrum hue cycle"},
	{headerPulse, "cyan pulse", "eases between light and dark cyan"},
	{headerAmber, "amber shimmer", "single amber/gold brightness wave"},
	{headerMagenta, "magenta shimmer", "single magenta brightness wave"},
	{headerOff, "off (static)", "no animation — static accent color"},
}

func headerStyleLabel(id string) string {
	for _, s := range headerStyles {
		if s.id == id {
			return s.label
		}
	}
	return id
}

func headerStyleItems() []pickerItem {
	items := make([]pickerItem, 0, len(headerStyles))
	for _, s := range headerStyles {
		items = append(items, pickerItem{id: s.id, title: s.label, subtitle: s.desc})
	}
	return items
}

// Animation frame rate (the header wordmark) in fps. Lower = fewer redraws =
// less CPU; the header style "off" stops the animation entirely (zero idle
// redraws). Persisted as settings.FPS.
const defaultFPS = 12

type fpsOption struct {
	fps         int
	label, desc string
}

var fpsOptions = []fpsOption{
	{24, "24 fps", "smoothest header animation — highest CPU"},
	{12, "12 fps", "smooth (default)"},
	{6, "6 fps", "calmer, lower CPU"},
	{3, "3 fps", "minimal CPU while still animating"},
}

func fpsLabel(fps int) string {
	for _, o := range fpsOptions {
		if o.fps == fps {
			return o.label
		}
	}
	return strconv.Itoa(fps) + " fps"
}

func fpsItems() []pickerItem {
	items := make([]pickerItem, 0, len(fpsOptions))
	for _, o := range fpsOptions {
		items = append(items, pickerItem{id: strconv.Itoa(o.fps), title: o.label, subtitle: o.desc})
	}
	return items
}

// commitFPS applies the chosen animation rate and persists it. (For zero idle
// redraws, set the header animation itself to "off".)
func (m *model) commitFPS(id string) {
	fps, err := strconv.Atoi(id)
	if err != nil || fps <= 0 {
		return
	}
	m.settings.FPS = fps
	saveSettings(m.settings)
	m.add(entInfo, "→ animation: "+fpsLabel(fps))
}

// settingsItems is the top-level /settings menu: one row per setting, each
// showing its current value. Selecting a row opens that setting's picker.
func (m *model) settingsItems() []pickerItem {
	return []pickerItem{
		{id: "header", title: "header animation", subtitle: "current: " + headerStyleLabel(m.settings.Header)},
		{id: "banner", title: "banner", subtitle: "current: " + bannerLabel(m.settings.Banner) + " · the color theme sets this"},
		{id: "fps", title: "animation fps", subtitle: "current: " + fpsLabel(m.settings.FPS) + " · lower = less CPU"},
		{id: "theme", title: "color theme", subtitle: "current: " + themeLabel(m.settings.Theme)},
		{id: "diff", title: "diff style", subtitle: "current: " + diffLabel(m.settings.Diff)},
		{id: "sidebarpos", title: "sidebar position", subtitle: "current: " + sidebarLabel(m.settings.Sidebar)},
		{id: "bar", title: "compact bar", subtitle: "current: " + barLabel(m.settings.Bar) + " · the /compact progress animation"},
		{id: "sysprompt", title: "extra system prompt", subtitle: "current: " + sysPromptLabel(m.settings.SysPrompt) + sysPromptEditedNote(m.sysPromptEdited()) + " · restarts the session to apply"},
	}
}

// commitHeaderStyle applies the chosen header animation live and persists it.
// Called when the user presses Enter in the /settings header picker.
//
// Animation never changes the frame's height — that is settings.Banner's job —
// so this is a repaint and needs no resize.
func (m *model) commitHeaderStyle(id string) {
	m.headerStyle = id
	m.settings.Header = id
	saveSettings(m.settings)
	m.add(entInfo, "→ header: "+headerStyleLabel(id))
}

// commitTheme applies the chosen color theme live (rebuilding every style) and
// persists it. rebuild() refreshes the transcript's themed parts; the chrome
// repaints on the next frame.
func (m *model) commitTheme(id string) {
	applyTheme(id)
	m.settings.Theme = id

	// The theme owns whether the banner shows — see themeDef.banner. Every
	// theme states it, so switching toggles in both directions: away from a
	// bannerless theme brings the banner back. An earlier version let a theme
	// stay silent, which meant switching away from cinder left it hidden.
	//
	// It is announced rather than applied silently, because it changes the
	// frame's height and the user asked for a palette, not a resize.
	note := ""
	if want := bannerFor(id); want != m.settings.Banner {
		m.settings.Banner = want
		m.resizeViewport()
		note = " · banner: " + bannerLabel(want)
	}

	saveSettings(m.settings)
	m.rerender() // re-render the whole transcript in the new palette
	m.add(entInfo, "→ theme: "+themeLabel(id)+note)
}

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

// saveSettings writes settings.json best-effort; failures are silent (settings
// are a nicety, not load-bearing).
func saveSettings(s settings) {
	p := settingsPath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		_ = os.WriteFile(p, b, 0o644)
	}
}

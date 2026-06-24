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
)

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
		{id: "fps", title: "animation fps", subtitle: "current: " + fpsLabel(m.settings.FPS) + " · lower = less CPU"},
		{id: "theme", title: "color theme", subtitle: "current: " + themeLabel(m.settings.Theme)},
	}
}

// commitHeaderStyle applies the chosen header animation live and persists it.
// Called when the user presses Enter in the /settings header picker.
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
	saveSettings(m.settings)
	m.rebuild()
	m.add(entInfo, "→ theme: "+themeLabel(id))
}

// settings is the persisted user config. Small and forward-compatible: unknown
// fields are ignored on load, missing ones take their default.
type settings struct {
	Header string `json:"header"`
	Theme  string `json:"theme"`
	FPS    int    `json:"fps"` // header animation frame rate; 0 → defaultFPS on load
}

func defaultSettings() settings {
	return settings{Header: headerCyan, Theme: defaultTheme, FPS: defaultFPS}
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

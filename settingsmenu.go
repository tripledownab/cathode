// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import "strconv"

// ---- the /settings menu and what each row commits ----
//
// Every commit* function here changes one setting through model.commitSetting
// (settings.go), which owns the write.

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
	m.commitSetting(func(s *settings) { s.Banner = id })
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
	m.commitSetting(func(s *settings) { s.FPS = fps })
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
	m.commitSetting(func(s *settings) { s.Header = id })
	m.add(entInfo, "→ header: "+headerStyleLabel(id))
}

// commitTheme applies the chosen color theme live (rebuilding every style) and
// persists it. rebuild() refreshes the transcript's themed parts; the chrome
// repaints on the next frame.
func (m *model) commitTheme(id string) {
	applyTheme(id)

	// The theme owns whether the banner shows — see themeDef.banner. Every
	// theme states it, so switching toggles in both directions: away from a
	// bannerless theme brings the banner back. An earlier version let a theme
	// stay silent, which meant switching away from cinder left it hidden.
	//
	// It is announced rather than applied silently, because it changes the
	// frame's height and the user asked for a palette, not a resize.
	// Read before the commit, because commitSetting is what changes m.settings.
	banner := bannerFor(id)
	shown := banner != m.settings.Banner

	// Both fields in one mutator: a theme carries its banner setting, so writing
	// them one at a time could leave the file holding this theme with the
	// previous one's banner.
	m.commitSetting(func(s *settings) {
		s.Theme = id
		s.Banner = banner
	})

	note := ""
	if shown {
		m.resizeViewport()
		note = " · banner: " + bannerLabel(banner)
	}
	m.rerender() // re-render the whole transcript in the new palette
	m.add(entInfo, "→ theme: "+themeLabel(id)+note)
}

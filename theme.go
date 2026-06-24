package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// ansiBaseHex maps the base-16 ANSI indices the BBS palette uses to their
// conventional xterm RGB, so a palette color given as an ANSI index can still
// be resolved to RGB (e.g. for the theme-color header shimmer).
var ansiBaseHex = map[string]string{
	"0": "#000000", "8": "#7f7f7f", "9": "#ff0000", "10": "#00ff00", "11": "#ffff00",
	"12": "#5c5cff", "13": "#ff00ff", "14": "#00ffff", "15": "#ffffff",
}

// resolveColorful turns a palette color (a hex "#rrggbb" or an ANSI index) into
// a concrete RGB color, falling back to bright cyan if it can't be parsed.
func resolveColorful(c lipgloss.Color) colorful.Color {
	s := string(c)
	if strings.HasPrefix(s, "#") {
		if cc, err := colorful.Hex(s); err == nil {
			return cc
		}
	} else if hex, ok := ansiBaseHex[s]; ok {
		if cc, err := colorful.Hex(hex); err == nil {
			return cc
		}
	}
	cc, _ := colorful.Hex("#00ffff")
	return cc
}

// appName drives the banner wordmark. Change this one line (and the module
// line in go.mod) when you settle on a name.
const appName = "cath0d3"

// palette is the nine semantic colors (plus accent) every style is built from.
// Roles: cyan = primary chrome/borders; mag = lightbar + scrollbar thumb;
// yel = the "you" label; white = subtext + inverted-bar foreground; green =
// tools + diff adds; red = errors + diff dels; blue = diff gutter; gray = dim
// text; black = the dark background behind inverted bars; accent = scene tags
// and the static header.
type palette struct {
	cyan, mag, yel, white, green, red, blue, gray, black, accent lipgloss.Color
}

// defaultTheme is the original look: the base-16 ANSI colors on black, so it
// rides whatever the user's terminal palette is. The named themes below pin
// hex values from the corresponding VS Code themes.
const defaultTheme = "bbs"

var palettes = map[string]palette{
	"bbs":        {cyan: "14", mag: "13", yel: "11", white: "15", green: "10", red: "9", blue: "12", gray: "8", black: "0", accent: "#FFB000"},
	"dracula":    {cyan: "#8be9fd", mag: "#ff79c6", yel: "#f1fa8c", white: "#f8f8f2", green: "#50fa7b", red: "#ff5555", blue: "#bd93f9", gray: "#6272a4", black: "#282a36", accent: "#ffb86c"},
	"nord":       {cyan: "#88c0d0", mag: "#b48ead", yel: "#ebcb8b", white: "#eceff4", green: "#a3be8c", red: "#bf616a", blue: "#81a1c1", gray: "#4c566a", black: "#2e3440", accent: "#d08770"},
	"solarized":  {cyan: "#2aa198", mag: "#d33682", yel: "#b58900", white: "#eee8d5", green: "#859900", red: "#dc322f", blue: "#268bd2", gray: "#586e75", black: "#002b36", accent: "#cb4b16"},
	"tokyonight": {cyan: "#7dcfff", mag: "#bb9af7", yel: "#e0af68", white: "#c0caf5", green: "#9ece6a", red: "#f7768e", blue: "#7aa2f7", gray: "#565f89", black: "#1a1b26", accent: "#ff9e64"},
	"gruvbox":    {cyan: "#8ec07c", mag: "#d3869b", yel: "#fabd2f", white: "#ebdbb2", green: "#b8bb26", red: "#fb4934", blue: "#83a598", gray: "#928374", black: "#282828", accent: "#fe8019"},
	"onedark":    {cyan: "#56b6c2", mag: "#c678dd", yel: "#e5c07b", white: "#abb2bf", green: "#98c379", red: "#e06c75", blue: "#61afef", gray: "#5c6370", black: "#282c34", accent: "#d19a66"},
	"monokai":    {cyan: "#66d9ef", mag: "#ae81ff", yel: "#e6db74", white: "#f8f8f2", green: "#a6e22e", red: "#f92672", blue: "#66d9ef", gray: "#75715e", black: "#272822", accent: "#fd971f"},
	"catppuccin": {cyan: "#94e2d5", mag: "#cba6f7", yel: "#f9e2af", white: "#cdd6f4", green: "#a6e3a1", red: "#f38ba8", blue: "#89b4fa", gray: "#6c7086", black: "#1e1e2e", accent: "#fab387"},
	"github":     {cyan: "#39c5cf", mag: "#bc8cff", yel: "#e3b341", white: "#c9d1d9", green: "#3fb950", red: "#f85149", blue: "#58a6ff", gray: "#8b949e", black: "#0d1117", accent: "#f0883e"},
	"rosepine":   {cyan: "#9ccfd8", mag: "#c4a7e7", yel: "#f6c177", white: "#e0def4", green: "#31748f", red: "#eb6f92", blue: "#9ccfd8", gray: "#6e6a86", black: "#191724", accent: "#ebbcba"},
}

// themeDef is one row in the /settings color-theme picker.
type themeDef struct{ id, label, desc string }

var themes = []themeDef{
	{"bbs", "BBS (terminal ANSI)", "the original neon 16-color palette"},
	{"dracula", "Dracula", "purple & pink on dark slate"},
	{"nord", "Nord", "muted arctic blues"},
	{"solarized", "Solarized Dark", "low-contrast teal & amber"},
	{"tokyonight", "Tokyo Night", "soft neon on deep blue"},
	{"gruvbox", "Gruvbox Dark", "warm retro earth tones"},
	{"onedark", "One Dark", "Atom's classic dark"},
	{"monokai", "Monokai", "the Sublime Text classic"},
	{"catppuccin", "Catppuccin Mocha", "pastel on dark mauve"},
	{"github", "GitHub Dark", "GitHub's dark mode"},
	{"rosepine", "Rosé Pine", "muted rose & pine"},
}

func themeLabel(id string) string {
	for _, t := range themes {
		if t.id == id {
			return t.label
		}
	}
	return id
}

func themeItems() []pickerItem {
	items := make([]pickerItem, 0, len(themes))
	for _, t := range themes {
		items = append(items, pickerItem{id: t.id, title: t.label, subtitle: t.desc})
	}
	return items
}

// ---- active palette (set by applyTheme) ----
var (
	colCyan, colMag, colYel, colWhite, colGreen, colRed, colBlue, colGray, colBlack, colAccent lipgloss.Color
)

// applyTheme swaps the active palette and rebuilds every style from it. Safe to
// call at runtime — all render call sites read these package vars at call time,
// so the next frame reflects the change. Unknown ids fall back to the default.
func applyTheme(name string) {
	p, ok := palettes[name]
	if !ok {
		p = palettes[defaultTheme]
	}
	colCyan, colMag, colYel, colWhite, colGreen, colRed, colBlue, colGray, colBlack, colAccent =
		p.cyan, p.mag, p.yel, p.white, p.green, p.red, p.blue, p.gray, p.black, p.accent
	buildStyles()
}

// init builds the styles once with the default palette so they're valid before
// any render; newModel re-applies the persisted theme at startup.
func init() { applyTheme(defaultTheme) }

// ---- styles (rebuilt by buildStyles from the active palette) ----
var (
	cYou, cName, cTool, cDim, cErr      lipgloss.Style
	userBox, toolBox, approveBar        lipgloss.Style
	dAdd, dDel, dCtx, dHunk, dGutter    lipgloss.Style
	dTitle, dBox                        lipgloss.Style
	hdrBox, hdrName, hdrDeco, hdrSub    lipgloss.Style
	statusBar, scrollThumb, scrollTrack lipgloss.Style
)

// buildStyles rebuilds every palette-derived style. Styles living in other
// files (sidebar, status) are rebuilt through their own hooks.
func buildStyles() {
	// ---- transcript labels + boxes (double-line borders = CP437 BBS feel) ----
	cYou = lipgloss.NewStyle().Foreground(colYel).Bold(true)
	cName = lipgloss.NewStyle().Foreground(colCyan).Bold(true)
	cTool = lipgloss.NewStyle().Foreground(colGreen).Bold(true)
	cDim = lipgloss.NewStyle().Foreground(colGray)
	cErr = lipgloss.NewStyle().Foreground(colRed).Bold(true)

	userBox = lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder(), false, false, false, true).
		BorderForeground(colYel).PaddingLeft(1)
	toolBox = lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colGreen).Padding(0, 1)
	// approveBar is the selected-menu "lightbar": bright on the accent's complement.
	approveBar = lipgloss.NewStyle().Bold(true).
		Foreground(colWhite).Background(colMag).Padding(0, 1)

	// ---- diff card ----
	dAdd = lipgloss.NewStyle().Foreground(colGreen)
	dDel = lipgloss.NewStyle().Foreground(colRed)
	dCtx = lipgloss.NewStyle().Foreground(colGray)
	dHunk = lipgloss.NewStyle().Foreground(colCyan)
	dGutter = lipgloss.NewStyle().Foreground(colBlue)
	dTitle = lipgloss.NewStyle().Bold(true).Foreground(colBlack).Background(colCyan).Padding(0, 1)
	dBox = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(colCyan)

	// ---- header banner + status line ----
	hdrBox = lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(colCyan).Padding(0, 1)
	hdrName = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	hdrDeco = lipgloss.NewStyle().Foreground(colCyan)
	hdrSub = lipgloss.NewStyle().Foreground(colWhite)
	statusBar = lipgloss.NewStyle().Bold(true).Foreground(colBlack).Background(colCyan)
	scrollThumb = lipgloss.NewStyle().Foreground(colMag)
	scrollTrack = lipgloss.NewStyle().Foreground(colGray)

	buildSidebarStyles()
	buildStatusStyles()
}

// colAccent is the scene-tag color (the "535510n" / wordmark divider tags and
// the sidebar STATION header, via hdrName). Per-theme now; for the BBS palette
// it's amber/gold. The magenta still drives the approve lightbar and scrollbar.

package main

import (
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// rainbowTickMsg advances the header wordmark's animation phase. It runs on its
// own cadence (independent of the spinner / splash ticks) so the logo keeps
// animating even when Claude is idle.
type rainbowTickMsg struct{}

// rainbowTick schedules the next animation frame at the configured fps. Lower
// fps means fewer idle redraws (less CPU); the header style "off" stops the
// loop entirely (the Update handler declines to re-arm it).
func rainbowTick(fps int) tea.Cmd {
	if fps <= 0 {
		fps = defaultFPS
	}
	return tea.Tick(time.Second/time.Duration(fps), func(time.Time) tea.Msg { return rainbowTickMsg{} })
}

// Tuning knobs for the sweep. charStep is the phase gap between adjacent letters
// (how much of the band is visible across the word at once); phaseStep is phase
// added per tick (how fast the band drifts). Degrees of the wave; 9°/tick at
// 80ms ≈ a full cycle every ~3.2s.
const (
	rainbowCharStep  = 26
	rainbowPhaseStep = 9
)

// shimmerMinV/MaxV bound the brightness swing of the single-hue "shimmer"
// styles. maxV 1.0 hits the fully-saturated hue (e.g. #00FFFF for cyan).
const (
	shimmerMinV = 0.35
	shimmerMaxV = 1.0
)

// renderHeader colors s for the header wordmark per the chosen style, offset by
// phase so the animated styles drift each tick. Chrome only — the header
// wordmark — never transcript content. Unknown styles fall back to cyan.
func renderHeader(style, s string, phase int) string {
	switch style {
	case headerRainbow:
		return paintHeader(s, phase, hueShade)
	case headerPulse:
		return paintHeader(s, phase, pulseShade)
	case headerAmber:
		return paintHeader(s, phase, amberShade)
	case headerMagenta:
		return paintHeader(s, phase, magentaShade)
	case headerTheme:
		return paintHeader(s, phase, themeShade)
	case headerOff:
		return hdrName.Render(s) // static, rides colAccent
	default: // headerCyan
		return paintHeader(s, phase, cyanShade)
	}
}

// themeShade shimmers in the active theme's primary color (colCyan — the same
// color as the side ornaments): it holds that color's hue + saturation and
// waves only its brightness, peaking at the full color so the wordmark meets
// the ornaments. Re-reads colCyan each call, so it follows theme changes live.
func themeShade(p float64) string {
	h, sat, v := resolveColorful(colCyan).Hsv()
	t := (math.Sin(p*math.Pi/180) + 1) / 2
	return colorful.Hsv(h, sat, v*(shimmerMinV+t*(shimmerMaxV-shimmerMinV))).Hex()
}

// paintHeader applies a per-character color (from shade, fed the drifting phase)
// across s. We lean on lipgloss to emit the SGR for each rune.
func paintHeader(s string, phase int, shade func(float64) string) string {
	var b strings.Builder
	i := 0
	for _, c := range s {
		p := float64(phase*rainbowPhaseStep + i*rainbowCharStep)
		st := lipgloss.NewStyle().Foreground(lipgloss.Color(shade(p))).Bold(true)
		b.WriteString(st.Render(string(c)))
		i++
	}
	return b.String()
}

// shimmer holds a single hue at full saturation and waves only the brightness
// along a sine of p (so the cycle eases in/out at each end). go-colorful — the
// same library lipgloss and the bubbles Progress gradient use — does HSV→RGB.
func shimmer(hue, p float64) string {
	t := (math.Sin(p*math.Pi/180) + 1) / 2
	v := shimmerMinV + t*(shimmerMaxV-shimmerMinV)
	return colorful.Hsv(hue, 1, v).Hex()
}

func cyanShade(p float64) string    { return shimmer(180, p) } // matches the chrome's bright cyan
func amberShade(p float64) string   { return shimmer(41, p) }  // amber/gold, near colAccent
func magentaShade(p float64) string { return shimmer(300, p) }

// hueShade is the original full-spectrum rainbow: cycle the hue at full S/V.
func hueShade(p float64) string {
	return colorful.Hsv(math.Mod(math.Mod(p, 360)+360, 360), 1, 1).Hex()
}

// pulseShade eases between a light and dark cyan in Lab space, for a gentler
// two-tone breathe rather than a hard dark↔bright swing.
func pulseShade(p float64) string {
	t := (math.Sin(p*math.Pi/180) + 1) / 2
	dark, _ := colorful.Hex("#0E5C63")
	light, _ := colorful.Hex("#AEEEEE")
	return dark.BlendLab(light, t).Clamped().Hex()
}

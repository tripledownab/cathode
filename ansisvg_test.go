package main

// ansiToSVG converts a rendered screen (TrueColor SGR) into a standalone SVG at
// a fixed monospace geometry — the engine behind TestGenerateAssets. Kept apart
// from the scene composition (asset_gen_test.go) so each file stays one concern.
// It lives in a _test.go file because it's tooling, not part of the app binary.

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

const (
	svgX0    = 16.0 // left margin
	svgY0    = 30.0 // first-line baseline
	svgCellW = 9.1  // monospace cell width
	svgLineH = 19.0 // line advance
	svgFont  = `font-family="ui-monospace,'DejaVu Sans Mono',Menlo,Consolas,monospace" font-size="15" xml:space="preserve"`
)

type svgRun struct {
	col    int
	text   string
	fg, bg string
	bold   bool
}

// ansiToSVG renders screen onto a canvas of bg, with unstyled text drawn in
// defaultFG (both hex, e.g. the active palette's base + text colors).
func ansiToSVG(screen, bg, defaultFG string) string {
	lines := strings.Split(strings.TrimRight(screen, "\n"), "\n")
	maxCols := 0
	for _, ln := range lines {
		if w := lipgloss.Width(ln); w > maxCols {
			maxCols = w
		}
	}
	width := int(svgX0*2 + float64(maxCols)*svgCellW)
	height := int(svgY0 + svgLineH*float64(len(lines)-1) + 14)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", width, height, width, height)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="%s"/>`+"\n", width, height, bg)
	for li, ln := range lines {
		y := svgY0 + svgLineH*float64(li)
		runs := parseRuns(ln)
		for _, r := range runs { // background fills go under the text
			if r.bg == "" {
				continue
			}
			x := svgX0 + float64(r.col)*svgCellW
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`+"\n",
				x, y-14, float64(lipgloss.Width(r.text))*svgCellW, svgLineH, r.bg)
		}
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" %s>`+"\n", svgX0, y, svgFont)
		for _, r := range runs {
			if strings.TrimSpace(r.text) == "" {
				continue // blank cells: the bg rect (if any) already drew them
			}
			fg := r.fg
			if fg == "" {
				fg = defaultFG
			}
			weight := 400
			if r.bold {
				weight = 700
			}
			fmt.Fprintf(&b, `<tspan x="%.1f" fill="%s" font-weight="%d">%s</tspan>`+"\n",
				svgX0+float64(r.col)*svgCellW, fg, weight, svgEscape(r.text))
		}
		b.WriteString("</text>\n")
	}
	b.WriteString("</svg>")
	return b.String()
}

// parseRuns splits one ANSI line into maximal same-style runs, tracking the
// visible column where each begins.
func parseRuns(line string) []svgRun {
	var runs []svgRun
	col, runStart := 0, 0
	var sb strings.Builder
	fg, bg := "", ""
	bold := false
	emit := func() {
		if sb.Len() > 0 {
			runs = append(runs, svgRun{col: runStart, text: sb.String(), fg: fg, bg: bg, bold: bold})
			col += lipgloss.Width(sb.String())
			sb.Reset()
		}
		runStart = col
	}
	for i := 0; i < len(line); {
		if line[i] == 0x1b && i+1 < len(line) && line[i+1] == '[' {
			j := i + 2
			for j < len(line) && line[j] != 'm' {
				j++
			}
			emit()
			applySGR(line[i+2:j], &fg, &bg, &bold)
			i = j + 1
			continue
		}
		r, sz := utf8.DecodeRuneInString(line[i:])
		sb.WriteRune(r)
		i += sz
	}
	emit()
	return runs
}

// applySGR mutates the running color/weight state from one "ESC[ ... m" body.
func applySGR(params string, fg, bg *string, bold *bool) {
	if params == "" {
		*fg, *bg, *bold = "", "", false
		return
	}
	toks := strings.Split(params, ";")
	for k := 0; k < len(toks); k++ {
		switch toks[k] {
		case "0", "":
			*fg, *bg, *bold = "", "", false
		case "1":
			*bold = true
		case "22":
			*bold = false
		case "39":
			*fg = ""
		case "49":
			*bg = ""
		case "38", "48":
			target := fg
			if toks[k] == "48" {
				target = bg
			}
			if k+4 < len(toks) && toks[k+1] == "2" { // 38;2;r;g;b
				*target = rgbHex(toks[k+2], toks[k+3], toks[k+4])
				k += 4
			} else if k+2 < len(toks) && toks[k+1] == "5" { // 38;5;n
				*target = xterm256Hex(toks[k+2])
				k += 2
			}
		}
	}
}

func rgbHex(r, g, b string) string {
	return fmt.Sprintf("#%02x%02x%02x", atoi(r), atoi(g), atoi(b))
}

// xterm256Hex maps a 256-color index to hex (cube + grayscale + a basic 16).
func xterm256Hex(s string) string {
	n := atoi(s)
	switch {
	case n >= 232: // grayscale ramp
		v := 8 + (n-232)*10
		return fmt.Sprintf("#%02x%02x%02x", v, v, v)
	case n >= 16: // 6x6x6 cube
		n -= 16
		conv := func(c int) int {
			if c == 0 {
				return 0
			}
			return 55 + c*40
		}
		return fmt.Sprintf("#%02x%02x%02x", conv(n/36), conv((n/6)%6), conv(n%6))
	default: // 0..15 — approximate with the cube's primaries
		base := []string{"#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
			"#7f7f7f", "#ff0000", "#00ff00", "#ffff00", "#5c5cff", "#ff00ff", "#00ffff", "#ffffff"}
		return base[n%16]
	}
}

func atoi(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }

func svgEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	return strings.ReplaceAll(s, ">", "&gt;")
}

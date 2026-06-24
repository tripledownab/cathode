package main

import (
	"strings"
	"unicode"
)

// ---- l33t text, StUdLy caps, and scene ornaments ----
// These run only on chrome/flavor text (banner, dividers, status, labels) —
// never on Claude's replies or the diff code, which must stay readable.

var leetRepl = strings.NewReplacer(
	"a", "4", "A", "4",
	"e", "3", "E", "3",
	"o", "0", "O", "0",
	"i", "1", "I", "1",
	"s", "5", "S", "5",
	"t", "7", "T", "7",
)

// leet does a moderate, still-legible numeral substitution.
func leet(s string) string { return leetRepl.Replace(s) }

// studly alternates case, scene "StUdLy cApS" style (deterministic).
func studly(s string) string {
	var b strings.Builder
	up := true
	for _, c := range s {
		if unicode.IsLetter(c) {
			if up {
				b.WriteRune(unicode.ToUpper(c))
			} else {
				b.WriteRune(unicode.ToLower(c))
			}
			up = !up
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// flavor = leet + studly, the full scene treatment for chrome text.
func flavor(s string) string { return studly(leet(s)) }

// scene ornament glyphs (CP437 / extended)
const (
	ornBullet = "▪"
	ornDot    = "°"
	ornCross  = "┼"
	ornDeco   = "·°·"
)

// sceneDivider renders an NFO/ad-style separator:  ··──┼──[ TAG ]──┼──··
func sceneDivider(plainLabel string, width int) string {
	plainTag := "[ " + plainLabel + " ]"
	fixed := 2 + 1 + len(plainTag) + 1 + 2 // ·· ┼ tag ┼ ··
	rails := width - fixed
	if rails < 2 {
		rails = 2
	}
	left, right := rails/2, rails-rails/2
	cy := func(s string) string { return dHunk.Render(s) }
	tag := cy("[ ") + hdrName.Render(plainLabel) + cy(" ]")
	return cy("··"+strings.Repeat("─", left)+"┼") + tag + cy("┼"+strings.Repeat("─", right)+"··")
}

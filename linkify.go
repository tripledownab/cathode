package main

import (
	"regexp"
	"strings"
)

// linkify wraps http(s) URLs in already-rendered transcript text with an OSC 8
// hyperlink, so terminals that support it make the URL clickable. The visible
// text stays the URL itself and the OSC 8 escape is zero-width, so layout is
// unchanged (verified against lipgloss width/truncation).
//
// Mouse note: while mouse capture is on (the default), most terminals follow an
// OSC 8 link on Cmd/Ctrl-click; with /mouse off a plain click works.
//
// OSC 8 form: ESC ] 8 ; ; <uri> BEL  <text>  ESC ] 8 ; ; BEL
const (
	oscOpen  = "\x1b]8;;"
	oscClose = "\x07"
)

// urlRe matches an http(s) URL up to whitespace or an escape byte, so the SGR
// codes Glamour wraps around a link aren't swallowed into the URI.
var urlRe = regexp.MustCompile(`https?://[^\s\x1b]+`)

func linkify(s string) string {
	if !strings.Contains(s, "://") {
		return s // fast path: nothing to do (also the asset-gen / no-URL case)
	}
	return urlRe.ReplaceAllStringFunc(s, func(u string) string {
		// Keep trailing prose punctuation out of the link target.
		trail := ""
		for len(u) > 0 && strings.IndexByte(".,;:!?)]}>'\"", u[len(u)-1]) >= 0 {
			trail = u[len(u)-1:] + trail
			u = u[:len(u)-1]
		}
		if u == "" {
			return trail
		}
		return oscOpen + u + oscClose + u + oscOpen + oscClose + trail
	})
}

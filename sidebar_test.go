package main

import (
	"strings"
	"testing"
)

// The sidebar sits on the configured side of the transcript.
func TestSidebarPosition(t *testing.T) {
	render := func(pos string) string {
		m := &model{sidebar: true, w: 100, mode: "ask", session: "a1b2c3d4"}
		m.settings.Sidebar = pos
		m.vp = newTranscriptViewport(100-1-sidebarWidth, 4)
		m.ready = true
		m.makeRenderer()
		m.entries = []entry{{kind: entClaude, text: "the reply"}}
		m.rebuild()
		return stripANSI(strings.SplitN(m.renderBody(), "\n", 2)[0]) // first row
	}

	right := render(sidebarRight)
	if strings.Index(right, "ClAuDe") > strings.Index(right, "StAtIoN") {
		t.Errorf("right: sidebar should follow the transcript:\n%s", right)
	}

	left := render(sidebarLeft)
	if strings.Index(left, "StAtIoN") > strings.Index(left, "ClAuDe") {
		t.Errorf("left: sidebar should precede the transcript:\n%s", left)
	}
}

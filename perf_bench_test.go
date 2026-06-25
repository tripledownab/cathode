package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
)

// Throwaway probe for the "scrolling lags after a long session" report: build a
// realistic large transcript and time rebuild() (only on new-entry/resize) vs the
// per-frame renderBackground()/View() (every animation tick and every scroll).
// Whichever scales with N is the cost we leave running overnight.

func benchModel(n int) model {
	ti := textinput.New()
	ti.Focus()
	sp := spinner.New()
	sp.Spinner = bbsSpinner("scan")
	m := model{
		mode: "ask", session: "a1b2c3d4", modelID: "sonnet",
		headerStyle: headerTheme,
		ctxTokens:   24000, outTokens: 1200, ctxLimit: 200000,
		busy: false, follow: true, ready: true,
		input: ti, sp: sp,
		w: 100, h: 40,
	}
	m.vp = newTranscriptViewport(m.w-1, m.h-6)
	m.makeRenderer()
	for i := 0; i < n; i++ {
		switch i % 4 {
		case 0:
			m.entries = append(m.entries, entry{kind: entUser, text: fmt.Sprintf("question number %d about the code", i)})
		case 1:
			m.entries = append(m.entries, entry{kind: entClaude, text: fmt.Sprintf("Here is a **markdown** reply #%d with a list:\n\n- one\n- two\n- three\n\nand a `code` span.", i)})
		case 2:
			m.entries = append(m.entries, entry{kind: entTool, toolName: "Bash", toolInput: json.RawMessage(`{"command":"go test ./..."}`)})
		case 3:
			m.entries = append(m.entries, entry{kind: entDiff, diffs: []fileDiff{{file: "math.go", old: "func add(a, b int) int {\n\treturn a + b\n}", new: "func add(a, b, c int) int {\n\treturn a + b + c\n}"}}})
		}
	}
	m.rebuild()
	m.vp.GotoBottom()
	return m
}

// BenchmarkAddEntry is the real hot path: a transcript of n entries, then one
// more arrives and we rebuild — exactly what every streamed message triggers.
// With the per-entry cache this should be flat regardless of n; before it was
// O(n) (a full Glamour re-render of the whole transcript each message).
func BenchmarkAddEntry(b *testing.B) {
	for _, n := range []int{200, 1000, 4000} {
		base := benchModel(n)
		b.Run(fmt.Sprintf("entries=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				base.entries = append(base.entries, entry{kind: entClaude, text: "a **new** reply with `code`"})
				base.rebuild()
			}
		})
	}
}

func BenchmarkRebuild(b *testing.B) {
	for _, n := range []int{200, 1000, 4000} {
		m := benchModel(n)
		b.Run(fmt.Sprintf("entries=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				m.rebuild()
			}
		})
	}
}

// BenchmarkRenderSolo profiles a single steady model so a -memprofile attributes
// per-frame allocation to renderBackground (not to the one-time rebuild in setup).
// refreshBody once up front so this measures a real steady frame (cached body),
// i.e. what a keystroke or animation tick actually costs.
func BenchmarkRenderSolo(b *testing.B) {
	m := benchModel(800)
	m.refreshBody()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.renderBackground()
	}
}

func BenchmarkRenderFrame(b *testing.B) {
	for _, n := range []int{200, 1000, 4000} {
		m := benchModel(n)
		b.Run(fmt.Sprintf("entries=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = m.renderBackground()
			}
		})
	}
}

// BenchmarkScrollFrame mimics one wheel notch: move the offset, then repaint.
func BenchmarkScrollFrame(b *testing.B) {
	for _, n := range []int{200, 1000, 4000} {
		m := benchModel(n)
		b.Run(fmt.Sprintf("entries=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if i%2 == 0 {
					m.vp.LineUp(3)
				} else {
					m.vp.LineDown(3)
				}
				_ = m.renderBackground()
			}
		})
	}
}

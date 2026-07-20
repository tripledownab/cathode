package main

import (
	"fmt"
	"strings"
	"testing"
)

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// The pending tray must never render more rows than its budget, and must always
// keep the "N queued" header so the count stays visible even when truncated.
func TestPendingTrayHonorsBudget(t *testing.T) {
	for _, n := range []int{1, 2, 5, 6, 20} {
		queue := make([]string, n)
		for i := range queue {
			queue[i] = fmt.Sprintf("queued message %d with some text", i)
		}
		for budget := 0; budget <= trayWant(n)+2; budget++ {
			tray := pendingTray(queue, 80, budget)
			got := lineCount(tray)
			if got > budget {
				t.Errorf("n=%d budget=%d: tray drew %d rows > budget", n, budget, got)
			}
			if got != trayRows(n, budget) {
				t.Errorf("n=%d budget=%d: tray drew %d rows, trayRows says %d", n, budget, got, trayRows(n, budget))
			}
			if got > 0 && !strings.Contains(tray, fmt.Sprintf("%d queued", n)) {
				t.Errorf("n=%d budget=%d: tray dropped the queued count:\n%s", n, budget, tray)
			}
		}
	}
}

// The composed frame must fit the terminal height for any queue depth, at every
// window size that can still hold the chrome — the queued strip never laps over
// the chat (the reported bug: viewport clamped to 1 while the tray overran).
func TestFrameFitsWithQueue(t *testing.T) {
	for _, h := range []int{14, 16, 20, 30, 40} {
		for _, n := range []int{0, 1, 3, 6, 12} {
			m := newModel(&Engine{}, "ask", nil, "bar", "")
			m.w, m.h = 80, h
			m.setPromptWidth(m.w - 4)
			m.resizeViewport()
			m.makeRenderer()
			for i := 0; i < 80; i++ {
				m.add(entClaude, "streamed assistant output line")
			}
			m.busy = true
			for i := 0; i < n; i++ {
				m.queue = append(m.queue, "queued message text here")
			}
			m.resizeViewport()
			m.refreshBody()
			got := lineCount(m.renderBackground())
			if got > h {
				t.Errorf("h=%d queue=%d: frame %d lines > terminal %d", h, n, got, h)
			}
			if m.vp.Height < 1 {
				t.Errorf("h=%d queue=%d: viewport starved to %d rows", h, n, m.vp.Height)
			}
		}
	}
}

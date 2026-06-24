package main

import "testing"

// The animation ticks are armed lazily so an idle screen stops redrawing. These
// lock the gating: a static/splash header never ticks, the spinner only ticks
// while busy, and neither double-arms.

func TestShouldAnimateHeader(t *testing.T) {
	cases := []struct {
		splash bool
		style  string
		fps    int
		want   bool
	}{
		{true, headerCyan, 12, false}, // splash up
		{false, headerOff, 12, false}, // static
		{false, headerCyan, 0, false}, // fps off
		{false, headerCyan, 12, true},
		{false, headerTheme, 3, true},
	}
	for _, c := range cases {
		m := &model{splash: c.splash, headerStyle: c.style}
		m.settings.FPS = c.fps
		if got := m.shouldAnimateHeader(); got != c.want {
			t.Errorf("splash=%v style=%q fps=%d: got %v want %v", c.splash, c.style, c.fps, got, c.want)
		}
	}
}

func TestArmHeaderIfNeeded(t *testing.T) {
	m := &model{headerStyle: headerCyan}
	m.settings.FPS = 12
	if c := m.armHeaderIfNeeded(); c == nil || !m.animating {
		t.Fatal("first arm should return a Cmd and set animating")
	}
	if c := m.armHeaderIfNeeded(); c != nil {
		t.Fatal("must not arm a second tick while one is in flight")
	}

	off := &model{headerStyle: headerOff}
	off.settings.FPS = 12
	if c := off.armHeaderIfNeeded(); c != nil || off.animating {
		t.Fatal("a static header must not arm a tick")
	}
}

func TestArmSpinnerIfNeeded(t *testing.T) {
	m := &model{}
	if c := m.armSpinnerIfNeeded(); c != nil || m.spinning {
		t.Fatal("idle (not busy) must not arm the spinner")
	}
	m.busy = true
	if c := m.armSpinnerIfNeeded(); c == nil || !m.spinning {
		t.Fatal("busy should arm the spinner once")
	}
	if c := m.armSpinnerIfNeeded(); c != nil {
		t.Fatal("must not double-arm the spinner")
	}
}

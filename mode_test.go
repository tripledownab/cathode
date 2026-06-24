package main

import "testing"

// TestNextModeCycle pins the Ctrl-T cycle: plan → ask → build → plan, ascending
// the autonomy ladder. bypass is intentionally absent; if it ever needs to be
// added, update the Ctrl-T handler in ui.go alongside.
func TestNextModeCycle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plan", "ask"},
		{"ask", "build"},
		{"build", "plan"},
		{"", "ask"},       // fallback for unset
		{"bypass", "ask"}, // not on the wheel — caller short-circuits, but the helper still has a deterministic answer
	}
	for _, c := range cases {
		if got := nextMode(c.in); got != c.want {
			t.Errorf("nextMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

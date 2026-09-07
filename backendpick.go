// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
)

// The backends -backend accepts. Named constants because the value is compared
// in more than one place (main gates the approvals server on it too), and a
// typo in a string literal there fails open: the server starts, nothing routes
// through it, and every gated tool waits for an approval that never arrives.
const (
	backendClaude = "claude"
	backendCodex  = "codex"
)

// startEngine spawns the backend the user asked for.
//
// The two are not symmetric, and the asymmetry is all in this function so the
// rest of the program does not carry it. claude takes its whole session shape
// as launch flags, which is why EngineConfig is already assembled by the time
// we get here. codex takes almost none as flags: the mode, model, working root
// and resumed thread are parameters of thread/start and turn/start, so they are
// handed to the engine instead and applied per call (codexcalls.go).
func startEngine(backend string, cfg EngineConfig, mode, resume, model string) (Engine, error) {
	switch backend {
	case backendClaude:
		e, err := newClaudeEngine(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to start claude: %w\nis the `claude` CLI installed and on PATH, and have you run `claude login`?", err)
		}
		return e, nil

	case backendCodex:
		cwd, _ := os.Getwd()
		e, err := newCodexEngine(codexEngineConfig{
			Model:    model,
			Mode:     mode,
			Cwd:      cwd,
			ResumeID: resume,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to start codex: %w\nis the `codex` CLI installed and on PATH, and have you run `codex login`?", err)
		}
		return e, nil
	}
	return nil, fmt.Errorf("unknown -backend %q: use %s or %s", backend, backendClaude, backendCodex)
}

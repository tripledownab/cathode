// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

// modeToPermission maps a friendly mode name to claude's --permission-mode.
// "plan" previews without touching files (OpenCode's read-only Plan mode);
// "build" auto-accepts edits; "ask" leaves the default gated behaviour, which
// is where the inline approval pane does its work.
func modeToPermission(mode string) string {
	switch mode {
	case "plan":
		return "plan"
	case "build":
		return "acceptEdits"
	case "bypass":
		return "bypassPermissions"
	default:
		return "default"
	}
}

// modeLabel maps an internal mode name to the on-screen label that mirrors
// Claude Code's UX: PLAN / EDIT / AUTO instead of plan / ask / build. We keep
// the internal names for the -mode flag (backwards-compatible) and only diverge
// at the render boundary.
func modeLabel(mode string) string {
	switch mode {
	case "plan":
		return "PLAN"
	case "ask":
		return "EDIT"
	case "build":
		return "AUTO"
	case "bypass":
		return "BYPASS"
	default:
		return "EDIT"
	}
}

// nextMode is the in-session Shift+Tab cycle. Order goes plan → ask → build →
// plan, climbing the autonomy ladder. bypass is deliberately not on the wheel:
// see the Shift+Tab handler in ui.go for why.
func nextMode(cur string) string {
	switch cur {
	case "plan":
		return "ask"
	case "ask":
		return "build"
	case "build":
		return "plan"
	default:
		return "ask"
	}
}

func main() {
	mode := flag.String("mode", "build", "ask | plan | build | bypass")
	mcp := flag.String("mcp", "", "path to a .mcp.json that wires your internal tools")
	modelID := flag.String("model", "", "pin a model (e.g. sonnet); empty uses account default")
	spin := flag.String("spinner", "bar", "working throbber: bar | shade | block | arrow | scan")
	dbg := flag.String("debug", "", "tee raw stream-json and MCP traffic to this logfile")
	resume := flag.String("resume", "", "claude session id to resume (also set automatically when picking from Ctrl-R)")
	ctx := flag.String("ctx", "200k", "context window for the pressure gauge — \"200k\", \"500k\", \"1m\", or a raw token count. auto-grows if observed input exceeds it.")
	flag.Parse()

	if *dbg != "" {
		d, err := OpenDebugLog(*dbg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "debug log disabled:", err)
		} else {
			debug = d
			defer debug.Close()
		}
	}

	// Start the in-process approval server unless we're in bypass mode (where
	// nothing is gated, so there's nothing to approve).
	var approvals *Approvals
	if *mode != "bypass" {
		if a, err := StartApprovals(); err == nil {
			approvals = a
		} else {
			fmt.Fprintln(os.Stderr, "approvals disabled:", err)
		}
	}

	cfg := EngineConfig{
		Model:          *modelID,
		PermissionMode: modeToPermission(*mode),
		MCPConfigPath:  *mcp,
	}
	// The user's standing instructions, when the toggle is on. This is a launch
	// flag with no runtime equivalent, which is why /sysprompt restarts to
	// change it (sysprompt.go). A failure to write the style file costs the
	// style, not the session, so report it and keep going.
	sysArgs, err := sysPromptArgs(loadSettings().SysPrompt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "extra system prompt disabled:", err)
	}
	cfg.ExtraArgs = append(cfg.ExtraArgs, sysArgs...)
	if *resume != "" {
		// `--resume` is documented as `[value]` (optional value), which under
		// Commander.js requires the `=` form — space-separated parses the id
		// as a positional and silently degrades to "open picker", which in
		// -p mode just starts a new session.
		cfg.ExtraArgs = append(cfg.ExtraArgs, "--resume="+*resume)
	}
	if approvals != nil {
		cfg.ApprovalsMCPConfig = approvals.mcpConfigJSON()
		cfg.PermissionPromptTool = approvals.permissionToolName()
	}

	engine, err := NewEngine(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to start claude:", err)
		fmt.Fprintln(os.Stderr, "is the `claude` CLI installed and on PATH, and have you run `claude login`?")
		os.Exit(1)
	}

	m := newModel(engine, *mode, approvals, *spin, *resume)
	m.ctxLimit = parseTokenCount(*ctx)
	// A resumed session may already exceed the base limit; grow it now that the
	// -ctx flag has set the floor, so the gauge starts honest (see observeCtx).
	m.observeCtx(m.ctxTokens)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	go engine.Pipe(p)

	final, err := p.Run()
	// Tear down the subprocess here, not in the Update loop: Close() blocks on
	// Wait(), which deadlocks against the Pipe goroutine while the program is
	// live. Once Run() has returned, p.Send is a no-op so Pipe keeps draining
	// stdout and claude exits cleanly (see Engine.Close).
	engine.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tui error:", err)
		os.Exit(1)
	}
	if fm, ok := final.(model); ok && fm.resumeID != "" {
		args := buildResumeArgv(os.Args, fm.resumeID, fm.mode)
		// syscall.Exec calls execve(2) directly — no $PATH lookup — so
		// os.Args[0] (often the bare name "doorway") would resolve relative
		// to cwd. From the repo root that hits the doorway/ subdirectory and
		// returns EACCES. os.Executable() gives the real binary path.
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, "resume: cannot locate self:", err)
			os.Exit(1)
		}
		if err := syscall.Exec(exe, args, os.Environ()); err != nil {
			fmt.Fprintln(os.Stderr, "resume re-exec failed:", err)
			os.Exit(1)
		}
	}
}

// restartResuming asks main for a re-exec that comes back into the given
// session. The single way to request a restart: main re-execs whenever the
// final model carries a resumeID (see buildResumeArgv), and the caller must
// return the Cmd.
//
// What survives is asymmetric, and worth knowing before adding a third caller:
// claude reloads the *whole* session via --resume, so its context is intact,
// but cathode's on-screen transcript is rehydrated from the session log and
// capped at replayMax entries (model.go). A long scrollback is shortened by a
// restart even though nothing was lost from the conversation itself.
func (m *model) restartResuming(session string) tea.Cmd {
	m.resumeID = session
	return tea.Quit
}

// buildResumeArgv rebuilds an argv that resumes the given session in the given
// mode, stripping any -resume / -mode already present so the new values win
// cleanly.
//
// Mode is carried across because it's mutable in-session (Shift+Tab, keys.go)
// while argv still holds whatever the process launched with. Without this a
// re-exec silently drops you back to the launch mode — which mattered little
// when only the session picker restarted, but /sysprompt made restarting a
// routine act (sysprompt.go).
func buildResumeArgv(orig []string, id, mode string) []string {
	out := []string{orig[0]}
	skipNext := false
	for _, a := range orig[1:] {
		if skipNext {
			skipNext = false
			continue
		}
		switch a {
		case "-resume", "--resume", "-mode", "--mode":
			skipNext = true
			continue
		}
		// The =-joined forms (-mode=plan) carry their value inline, so there's
		// no following argument to skip.
		if strings.HasPrefix(a, "-resume=") || strings.HasPrefix(a, "--resume=") ||
			strings.HasPrefix(a, "-mode=") || strings.HasPrefix(a, "--mode=") {
			continue
		}
		out = append(out, a)
	}
	out = append(out, "-resume", id)
	if mode != "" {
		out = append(out, "-mode", mode)
	}
	return out
}

// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
)

// TestScrubbedEnvDropsWhatClaudeMustNotInherit pins both halves of the rule:
// what has to go, and what has no business going with it.
//
// The credentials are the billing constraint — either present makes claude
// bill the API instead of the subscription. CLAUDE_CODE_CHILD_SESSION turns
// transcript saving off in the spawned session, which leaves the ids in the
// ctrl+r picker pointing at nothing for --resume to reload.
func TestScrubbedEnvDropsWhatClaudeMustNotInherit(t *testing.T) {
	gone := []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_CHILD_SESSION"}
	// KEY and TOKEN are named here because the scrub used to be a substring
	// test against the dropped names run together, so a variable whose name
	// was a tail of one of them disappeared as well.
	kept := []string{"ANTHROPIC_MODEL", "CLAUDE_CODE_ENTRYPOINT", "KEY", "TOKEN"}

	for _, name := range append(append([]string{}, gone...), kept...) {
		t.Setenv(name, "value-for-"+name)
	}

	survived := map[string]bool{}
	for _, kv := range scrubbedEnv() {
		if name, _, ok := strings.Cut(kv, "="); ok {
			survived[name] = true
		}
	}
	for _, name := range gone {
		if survived[name] {
			t.Errorf("%s survived the scrub", name)
		}
	}
	for _, name := range kept {
		if !survived[name] {
			t.Errorf("the scrub removed %s, which it has no business dropping", name)
		}
	}
	if len(scrubbedEnv()) >= len(os.Environ()) {
		t.Error("scrubbedEnv did not drop anything")
	}
}

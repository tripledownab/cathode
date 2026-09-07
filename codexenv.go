// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
)

// codexDropFromEnv is what the spawned codex must not inherit.
//
// Unlike claude, an OPENAI_API_KEY in the environment does NOT divert billing:
// codex reads its credential from ~/.codex/auth.json, and a probe confirmed it
// still resolves auth mode "chatgpt" with the variable set, and even with
// preferred_auth_method forced to apikey. Only `codex login --with-api-key`
// puts a key where it counts. It is dropped anyway, because the cost is nothing
// and the claim above is a fact about one version.
//
// OPENAI_BASE_URL is the one that matters here. It is not a billing lever, it
// is a routing one: it decides which host the conversation is sent to.
var codexDropFromEnv = map[string]bool{
	"OPENAI_API_KEY":  true,
	"OPENAI_BASE_URL": true,
}

// codexEnv returns the environment to spawn codex with. Whole-name matching,
// for the reason scrubbedEnv documents: a substring test also drops any
// variable whose name merely ends with one of these.
func codexEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if name, _, ok := strings.Cut(kv, "="); ok && codexDropFromEnv[name] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// claudeRecord is the minimal shape we need from one line of claude's session
// JSONL. Each turn writes a {"type":"user"|"assistant", "message":{...}} record;
// many other record types (queue-operation, summary, hook events) get ignored.
// Content stays raw because claude writes it two ways — see contentBlocks.
type claudeRecord struct {
	Type string `json:"type"`
	// isMeta marks context claude injected into the conversation (a skill
	// preamble, the local-command caveat); isCompactSummary marks the summary
	// a compaction wrote. Neither is the user talking — see replay.go.
	IsMeta           bool `json:"isMeta"`
	IsCompactSummary bool `json:"isCompactSummary"`
	Message          *struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
		// usage is present on assistant records; its input + cache totals give
		// the context size at that turn, which we replay into the gauge on
		// resume (see loadPriorTranscript / model.go).
		Usage *Usage `json:"usage"`
	} `json:"message"`
}

// contentBlock is one element of a message's content array.
type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// contentBlocks decodes both encodings claude uses for message.content: an
// array of typed blocks, or a bare string for a plain typed prompt. Decoding
// the bare-string form into a []contentBlock fails, and the record it belongs
// to is the user's own prompt — so treating that as "unparseable, skip" dropped
// half the conversation out of a replayed transcript.
func contentBlocks(raw json.RawMessage) []contentBlock {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		return []contentBlock{{Type: "text", Text: s}}
	}
	var blocks []contentBlock
	_ = json.Unmarshal(raw, &blocks)
	return blocks
}

// sessionTranscriptPath returns the on-disk path to claude's JSONL transcript
// for the given session, scoped to the current cwd. Returns "" + error if the
// home dir or cwd can't be resolved.
func sessionTranscriptPath(sessionID string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir, err := claudeProjectDir(cwd)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sessionID+".jsonl"), nil
}

// loadPriorTranscript reads claude's persisted session for sessionID and
// projects it into Doorway entry kinds. Returns the last maxEntries entries
// (most recent at end) plus ctxTokens — the context size carried by the most
// recent assistant turn's usage (input + cache_read + cache_creation), used to
// seed the context gauge on resume so it isn't stuck at 0% until the first new
// turn. Returns (nil, 0) on any file error so a missing/corrupt session
// degrades to an empty transcript instead of crashing.
func loadPriorTranscript(sessionID string, maxEntries int) (entries []entry, ctxTokens int) {
	path, err := sessionTranscriptPath(sessionID)
	if err != nil {
		return nil, 0
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0
	}
	defer f.Close()

	// The summary a compaction writes is logged *before* the /compact record
	// that caused it, so the marker is held back and flushed once the command
	// line has printed — the order the live run shows. An auto-compaction has
	// no command records, and flushes at the next reply or at EOF.
	pendingCompact := false
	flushCompact := func() {
		if pendingCompact {
			entries = append(entries, entry{kind: entInfo, text: compactDoneText})
			pendingCompact = false
		}
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxRecordBytes) // tool results can be large
	for sc.Scan() {
		var rec claudeRecord
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil || rec.Message == nil {
			continue
		}
		// A compaction reports as one line live, so replay it as one line too
		// rather than as the whole summary in a user box. Injected context is
		// dropped outright, the way the live stream drops it.
		if rec.IsCompactSummary {
			pendingCompact = true
			continue
		}
		if rec.IsMeta {
			continue
		}
		// The last assistant usage we see wins — it reflects the freshest
		// context size, the same number the live stream reports per turn.
		if rec.Type == "assistant" && rec.Message.Usage != nil {
			u := rec.Message.Usage
			ctxTokens = u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
		}
		for _, c := range contentBlocks(rec.Message.Content) {
			switch c.Type {
			case "text":
				if rec.Type == "user" {
					// The user side of the log carries the CLI's own bookkeeping
					// records alongside real prompts (replay.go).
					e, k := replayUserText(c.Text)
					switch k {
					case replayShow:
						entries = append(entries, e)
					case replayStdout:
						// /compact's stdout says "Compacted" — the same event the
						// held marker reports, so report it once, as the marker.
						if pendingCompact {
							e, pendingCompact = entry{kind: entInfo, text: compactDoneText}, false
						}
						entries = append(entries, e)
					}
					continue
				}
				flushCompact()
				if t := strings.TrimSpace(c.Text); t != "" {
					entries = append(entries, entry{kind: entClaude, text: t})
				}
			case "tool_use":
				flushCompact()
				if ds, ok := diffsForTool(c.Name, c.Input); ok {
					entries = append(entries, entry{kind: entDiff, diffs: ds})
				} else {
					entries = append(entries, entry{kind: entTool, toolName: c.Name, toolInput: c.Input})
				}
			}
		}
	}
	flushCompact() // a compaction with nothing after it still gets its line
	if maxEntries > 0 && len(entries) > maxEntries {
		entries = entries[len(entries)-maxEntries:]
	}
	return entries, ctxTokens
}

// Copyright 2026 Triple Down AB
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// fullTranscript sizes a model and fills it past one screen, so the newest
// line is visible only if the viewport is pinned to the real bottom.
func fullTranscript(t *testing.T) model {
	t.Helper()
	m, _ := newTestModel(t, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = mm.(model)
	for i := 0; i < 60; i++ {
		m.add(entInfo, fmt.Sprintf("filler %d", i))
	}
	return m
}

func lastLineShows(m model) bool {
	return strings.Contains(ansi.Strip(m.vp.View()), "END-MARKER")
}

// Glamour never breaks a token, so each of these reached the viewport wider
// than it. The viewport wrapped the line again and cut the overflow from the
// bottom, which is where the newest output is.
func TestWideReplyKeepsItsLastLineInView(t *testing.T) {
	long := strings.Repeat("x", 150)
	for name, md := range map[string]string{
		"code block":  "```go\nfmt.Println(\"" + long + "\")\n```",
		"url":         "see https://example.com/" + long,
		"inline code": "use `" + long + "` here",
		"path":        "edited /src/" + long + "/main.go",
		"wide runes":  strings.Repeat("字", 120),
		"tabbed code": "```go\nfunc f() {\n" + strings.Repeat("\t\t"+strings.Repeat("a", 70)+"\n", 3) + "}\n```",
	} {
		m := fullTranscript(t)
		m.add(entAgent, md+"\n\nEND-MARKER")
		if !lastLineShows(m) {
			t.Errorf("%s: the last line of the reply is cut off", name)
		}
		for _, l := range strings.Split(m.content.String(), "\n") {
			if w := ansi.StringWidth(l); w > m.vp.Width {
				t.Errorf("%s: a content line is %d cells in a %d-cell viewport", name, w, m.vp.Width)
				break
			}
		}
	}
}

// A shorter viewport keeps its offset, so without a re-pin the rows it loses
// are the newest ones.
func TestShrinkingTheViewportKeepsTheNewestLineInView(t *testing.T) {
	m := fullTranscript(t)
	m.add(entInfo, "END-MARKER")
	m.startCompacting()
	m.input.SetValue("a\nb\nc\nd\ne\nf")
	m.syncPromptHeight()
	if !lastLineShows(m) {
		t.Fatal("the compact bar and a taller prompt pushed the newest line out of view")
	}
}

func TestFitWidthCarriesALinkAcrossABreak(t *testing.T) {
	url := "https://example.com/" + strings.Repeat("y", 30)
	got := strings.Split(fitWidth(linkify(url), 20), "\n")
	if len(got) < 2 {
		t.Fatalf("the link did not break: %q", got)
	}
	for i, line := range got {
		if !strings.HasPrefix(line, oscOpen+url+oscClose) {
			t.Errorf("line %d does not open the link: %q", i, line)
		}
		if openLink(line) != "" {
			t.Errorf("line %d leaves the link open: %q", i, line)
		}
	}
}

// A break inside a styled token leaves its colour open, and the scrollbar
// cell joined after the line would take it.
func TestFitWidthClosesTheStyleOfABrokenLine(t *testing.T) {
	styled := "\x1b[48;5;236m" + strings.Repeat("c", 30) + "\x1b[0m"
	for i, line := range strings.Split(fitWidth(styled, 20), "\n") {
		if !strings.HasSuffix(line, ansi.ResetStyle) {
			t.Errorf("line %d ends with its style open: %q", i, line)
		}
	}
}

// Glamour measures a tab as no cells and pads by that measure. Given raw tabs,
// its padding overflowed once the tabs were expanded, and every tabbed line
// gained a blank row under it.
func TestTabbedCodeRendersLikeTheSameCodeInSpaces(t *testing.T) {
	code := func(indent string) string {
		return "```go\nfunc f() {\n" + indent + "return\n}\n```"
	}
	m := fullTranscript(t)
	tabbed := ansi.Strip(m.renderEntry(entry{kind: entAgent, text: code("\t")}))
	spaced := ansi.Strip(m.renderEntry(entry{kind: entAgent, text: code("    ")}))
	if fitWidth(tabbed, m.vp.Width) != fitWidth(spaced, m.vp.Width) {
		t.Errorf("tabbed code renders differently from spaced code:\n%q\n%q", tabbed, spaced)
	}
}

func TestFitWidthMeasuresATabAsTheViewportDraws(t *testing.T) {
	for i, line := range strings.Split(fitWidth("\t"+strings.Repeat("x", 18), 20), "\n") {
		if strings.Contains(line, "\t") || ansi.StringWidth(line) > 20 {
			t.Errorf("line %d still overflows once its tab is drawn: %q", i, line)
		}
	}
}

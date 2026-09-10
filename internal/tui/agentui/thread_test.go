package agentui

import (
	"strings"
	"testing"

	"github.com/matteo-psnt/termwise/internal/theme"
)

func testRenderer() Renderer { return newRenderer(true, theme.Get("forest")) }

// indentOf reports the leading-space count of a rendered line, ignoring colour.
func indentOf(line string) int {
	plain := stripANSI(line)
	return len(plain) - len(strings.TrimLeft(plain, " "))
}

// A new user message opens a new turn and gets a divider; the first one does not.
func TestThreadSeparatesTurnsButNotTheFirst(t *testing.T) {
	r := testRenderer()

	single := r.RenderThread([]ThreadEntry{
		UserEntry{Content: "first"},
		AssistantEntry{Content: "answer"},
	}, 72)
	if strings.Contains(stripANSI(single), "╌") {
		t.Errorf("a divider was drawn before the first turn:\n%s", stripANSI(single))
	}

	two := r.RenderThread([]ThreadEntry{
		UserEntry{Content: "first"},
		AssistantEntry{Content: "answer"},
		UserEntry{Content: "second"},
	}, 72)
	dividers := 0
	for _, ln := range strings.Split(stripANSI(two), "\n") {
		if t := strings.TrimSpace(ln); t != "" && strings.Trim(t, "╌") == "" {
			dividers++
		}
	}
	if dividers != 1 {
		t.Errorf("expected exactly one divider line between two turns, found %d:\n%s", dividers, stripANSI(two))
	}
}

// Tool activity must sit deeper than the turn it belongs to, so the answer is
// what the eye lands on.
func TestToolActivityIsIndentedBelowTheTurn(t *testing.T) {
	r := testRenderer()
	lines := strings.Split(r.RenderThread([]ThreadEntry{
		UserEntry{Content: "check the port"},
		ToolCallEntry{Name: "bash", Detail: "lsof -i :5173"},
		ToolResultEntry{Content: "node 41233\nnode 41240"},
		AssistantEntry{Content: "a vite server"},
	}, 72), "\n")

	user, call, result, assistant := -1, -1, -1, -1
	for _, ln := range lines {
		plain := stripANSI(ln)
		switch {
		case strings.Contains(plain, "›"):
			user = indentOf(ln)
		case strings.Contains(plain, "⏺"):
			call = indentOf(ln)
		case strings.Contains(plain, "⎿"):
			result = indentOf(ln)
		case strings.Contains(plain, "◆"):
			assistant = indentOf(ln)
		}
	}
	if user != 0 || assistant != 0 {
		t.Errorf("turn sigils should sit in the gutter: user=%d assistant=%d", user, assistant)
	}
	if call <= user {
		t.Errorf("tool call indent %d is not deeper than the turn gutter %d", call, user)
	}
	if result <= call {
		t.Errorf("tool result indent %d is not deeper than its call %d", result, call)
	}
}

// A wrapped or multi-line entry must stay inside the gutter it started in.
func TestMultiLineEntriesHangFromTheGutter(t *testing.T) {
	r := testRenderer()
	out := r.RenderThread([]ThreadEntry{
		UserEntry{Content: "line one\nline two\nline three"},
	}, 72)
	lines := strings.Split(stripANSI(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%q", len(lines), lines)
	}
	for i, ln := range lines[1:] {
		if got := indentOf(ln); got != gutterWidth {
			t.Errorf("continuation line %d indent = %d, want %d (%q)", i+1, got, gutterWidth, ln)
		}
	}
}

func TestTurnSeparatorFitsNarrowPanes(t *testing.T) {
	r := testRenderer()
	for _, w := range []int{10, 20, 72, 200} {
		sep := stripANSI(r.turnSeparator(w))
		if len([]rune(sep)) > max(w, 8) {
			t.Errorf("width %d: separator is %d wide", w, len([]rune(sep)))
		}
		if len([]rune(sep)) < 8 {
			t.Errorf("width %d: separator collapsed to %d", w, len([]rune(sep)))
		}
	}
}

func TestEmptyThreadRendersNothing(t *testing.T) {
	if got := testRenderer().RenderThread(nil, 72); got != "" {
		t.Errorf("RenderThread(nil) = %q", got)
	}
}

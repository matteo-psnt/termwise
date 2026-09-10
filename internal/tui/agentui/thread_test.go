package agentui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"

	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/theme"
)

func testRenderer() Renderer { return newRenderer(true, theme.Get("forest")) }

// indentOf reports the leading-space count of a rendered line, ignoring colour.
func indentOf(line string) int {
	plain := stripANSI(line)
	return len(plain) - len(strings.TrimLeft(plain, " "))
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

func TestEmptyThreadRendersNothing(t *testing.T) {
	if got := testRenderer().RenderThread(nil, 72); got != "" {
		t.Errorf("RenderThread(nil) = %q", got)
	}
}

// A gated bash command is announced once, when it is put up for approval.
// Executing it afterwards must not announce it again — the LLM judge path made
// every auto-approved command appear twice in the thread.
func TestGatedToolCallIsAnnouncedOnlyOnce(t *testing.T) {
	tc := provider.ToolCall{ID: "1", Name: "bash", Input: map[string]any{"command": "find src -name '*.js'"}}

	m := Model{}
	m.renderer = testRenderer()
	m.vp = viewport.New()

	// Put it up for approval, then execute it the way the approval path does.
	gotModel, _ := m.handleNeedsApprovalEvent(agent.NeedsApprovalEvent{ToolCall: tc, Command: "find src -name '*.js'"})
	m = gotModel.(Model)
	m.llmJudge = false
	gotModel, _ = m.handleToolExecutedEvent(agent.ToolExecutedEvent{
		ToolCall:     tc,
		Result:       provider.ToolResult{ToolCallID: "1", Content: "src/server.js"},
		AutoAccepted: false,
	})
	m = gotModel.(Model)

	calls := 0
	for _, e := range m.thread {
		if _, ok := e.(ToolCallEntry); ok {
			calls++
		}
	}
	if calls != 1 {
		t.Errorf("gated command announced %d times, want 1", calls)
	}
}

// An ungated call is never pre-announced, so executing it must announce it.
func TestAutoAcceptedToolCallIsAnnounced(t *testing.T) {
	m := Model{}
	m.renderer = testRenderer()
	m.vp = viewport.New()

	gotModel, _ := m.handleToolExecutedEvent(agent.ToolExecutedEvent{
		ToolCall:     provider.ToolCall{ID: "1", Name: "read", Input: map[string]any{"path": "go.mod"}},
		Result:       provider.ToolResult{ToolCallID: "1", Content: "module x"},
		AutoAccepted: true,
	})
	m = gotModel.(Model)

	calls := 0
	for _, e := range m.thread {
		if _, ok := e.(ToolCallEntry); ok {
			calls++
		}
	}
	if calls != 1 {
		t.Errorf("auto-accepted call announced %d times, want 1", calls)
	}
}

// Turns are separated by space, not by a drawn rule. The input area already
// has two full-width rules directly below the thread; a third one between
// every turn made the pane read as a stack of boxes.
func TestTurnsAreSeparatedWithoutARule(t *testing.T) {
	out := testRenderer().RenderThread([]ThreadEntry{
		UserEntry{Content: "first"},
		AssistantEntry{Content: "answer"},
		UserEntry{Content: "second"},
	}, 72)
	plain := stripANSI(out)

	for _, rule := range []string{"╌", "───", "---"} {
		if strings.Contains(plain, rule) {
			t.Errorf("a rule (%q) was drawn between turns:\n%s", rule, plain)
		}
	}
	// A new turn still gets air above it.
	if !strings.Contains(plain, "\n\n") {
		t.Errorf("no blank line separating turns:\n%q", plain)
	}
}

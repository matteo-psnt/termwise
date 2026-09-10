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

// A tool result nests under its call: the call sits flush with thread content,
// the corner glyph one step in, and continuation lines under the corner's text.
func TestToolResultNestsUnderItsCall(t *testing.T) {
	r := testRenderer()
	lines := strings.Split(r.RenderThread([]ThreadEntry{
		UserEntry{Content: "check the port"},
		ToolCallEntry{Name: "bash", Detail: "lsof -i :5173"},
		ToolResultEntry{Content: "one\ntwo"},
	}), "\n")

	call, corner, cont := -1, -1, -1
	for _, ln := range lines {
		plain := stripANSI(ln)
		switch {
		case strings.Contains(plain, "⏺"):
			call = indentOf(ln)
		case strings.Contains(plain, "⎿"):
			corner = indentOf(ln)
		case strings.Contains(plain, "two"):
			cont = indentOf(ln)
		}
	}
	if call != 0 {
		t.Errorf("tool call indent = %d, want 0 (flush with thread content)", call)
	}
	if corner != 2 {
		t.Errorf("result corner indent = %d, want 2", corner)
	}
	if cont != 5 {
		t.Errorf("result continuation indent = %d, want 5", cont)
	}
}

// A multi-line entry stays aligned under the text of its first line, not under
// the sigil.
func TestMultiLineEntriesHangFromTheSigil(t *testing.T) {
	lines := strings.Split(stripANSI(testRenderer().RenderThread([]ThreadEntry{
		UserEntry{Content: "line one\nline two\nline three"},
	})), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3: %q", len(lines), lines)
	}
	for i, ln := range lines[1:] {
		if got := indentOf(ln); got != 2 {
			t.Errorf("continuation line %d indent = %d, want 2 (%q)", i+1, got, ln)
		}
	}
}

func TestEmptyThreadRendersNothing(t *testing.T) {
	if got := testRenderer().RenderThread(nil); got != "" {
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

// glamour's stock style configs prefix H2-H6 with their literal markdown, which
// used to leak "## " into the rendered thread and forced a workaround into the
// system prompt.
func TestMarkdownHeadingsRenderWithoutLiteralHashes(t *testing.T) {
	for _, dark := range []bool{false, true} {
		r := newRenderer(dark, theme.Get("forest"))
		out := stripANSI(renderMarkdown("# One\n\n## Two\n\n### Three\n\nbody\n", r.glamour))
		for _, bad := range []string{"## ", "### "} {
			if strings.Contains(out, bad) {
				t.Errorf("dark=%v: rendered markdown still contains %q:\n%s", dark, bad, out)
			}
		}
	}
}

package agentui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/provider"
)

func TestTrimContextDropsOldToolResultsFirst(t *testing.T) {
	large := strings.Repeat("x", 2_000)
	messages := []provider.Message{
		{
			Role: "user",
			ToolResults: []provider.ToolResult{
				{ToolCallID: "1", Content: large},
				{ToolCallID: "2", Content: large},
			},
		},
		{
			Role:    "assistant",
			Content: "keep assistant text",
		},
	}

	trimmed := trimContext(messages, 2_000)
	if got := trimmed[0].ToolResults[0].Content; !strings.Contains(got, "result dropped") {
		t.Fatalf("expected oldest tool result to be dropped, got %q", got)
	}
	if got := trimmed[0].ToolResults[1].Content; strings.Contains(got, "result dropped") {
		t.Fatalf("expected trimming to stop after the oldest result, got %q", got)
	}
	if trimmed[1].Content != "keep assistant text" {
		t.Fatalf("expected assistant text to be preserved, got %q", trimmed[1].Content)
	}
}

func TestTrimContextLeavesMessagesWithinLimitUntouched(t *testing.T) {
	messages := []provider.Message{
		{Role: "user", Content: "short"},
		{Role: "assistant", Content: "also short"},
	}

	trimmed := trimContext(messages, 10_000)
	if trimmed[0].Content != "short" || trimmed[1].Content != "also short" {
		t.Fatalf("expected messages to remain unchanged, got %#v", trimmed)
	}
}

func TestHandleThinkingKeyEscInterruptsRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := Model{
		ctx:          ctx,
		cancel:       cancel,
		state:        stateThinking,
		activeTurnID: 7,
		vp:           viewport.New(80, 10),
	}

	gotModel, _ := m.handleThinkingKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := gotModel.(Model)

	if got.state != stateIdle {
		t.Fatalf("expected stateIdle after interrupt, got %v", got.state)
	}
	if got.activeTurnID != 0 {
		t.Fatalf("expected active turn to be cleared, got %d", got.activeTurnID)
	}
	if len(got.thread) != 1 {
		t.Fatalf("expected one thread entry, got %d", len(got.thread))
	}
	if entry, ok := got.thread[0].(ErrorEntry); !ok || entry.Content != "Request interrupted." {
		t.Fatalf("expected interrupt entry, got %#v", got.thread[0])
	}

	select {
	case <-ctx.Done():
	default:
		t.Fatal("expected the original context to be canceled")
	}
}

func TestHandleResponseMsgIgnoresStaleResponse(t *testing.T) {
	m := Model{
		state:        stateIdle,
		activeTurnID: 2,
		vp:           viewport.New(80, 10),
	}

	gotModel, _ := m.handleTurnMsg(turnMsg{
		turnID: 1,
		msg:    agent.ResponseMsg{Resp: &provider.ChatResponse{Content: "stale"}},
	})
	got := gotModel.(Model)

	if len(got.thread) != 0 {
		t.Fatalf("expected no error thread entry, got %#v", got.thread)
	}
	if got.activeTurnID != 2 {
		t.Fatalf("expected active turn to remain unchanged, got %d", got.activeTurnID)
	}
}

func TestHandleResponseMsgIgnoresCanceledActiveRequest(t *testing.T) {
	m := Model{
		state:        stateThinking,
		activeTurnID: 3,
		vp:           viewport.New(80, 10),
	}

	gotModel, _ := m.handleTurnMsg(turnMsg{
		turnID: 3,
		msg:    agent.ResponseMsg{Err: context.Canceled},
	})
	got := gotModel.(Model)

	if got.activeTurnID != 0 {
		t.Fatalf("expected active turn to be cleared, got %d", got.activeTurnID)
	}
	if len(got.thread) != 0 {
		t.Fatalf("expected no thread entry, got %#v", got.thread)
	}
}

func TestHandleToolExecutedMsgIgnoresStaleTurn(t *testing.T) {
	m := Model{
		activeTurnID: 2,
		vp:           viewport.New(80, 10),
	}

	gotModel, _ := m.handleTurnMsg(turnMsg{
		turnID: 1,
		msg: agent.ToolExecutedMsg{
			ToolCall: provider.ToolCall{Name: "bash", Input: map[string]any{"command": "echo stale"}},
			Result:   provider.ToolResult{ToolCallID: "1", Content: "stale"},
		},
	})
	got := gotModel.(Model)

	if len(got.thread) != 0 {
		t.Fatalf("expected stale tool result to be ignored, got %#v", got.thread)
	}
}

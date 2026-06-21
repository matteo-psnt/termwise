package agentui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
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
		ctx:              ctx,
		cancel:           cancel,
		state:            stateThinking,
		activeGeneration: 7,
		vp:               viewport.New(80, 10),
	}

	gotModel, _ := m.handleThinkingKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := gotModel.(Model)

	if got.state != stateIdle {
		t.Fatalf("expected stateIdle after interrupt, got %v", got.state)
	}
	if got.activeGeneration != 0 {
		t.Fatalf("expected active generation to be cleared, got %d", got.activeGeneration)
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
		state:            stateIdle,
		activeGeneration: 2,
		vp:               viewport.New(80, 10),
	}

	gotModel, _ := m.handleGenerationMsg(generationMsg{
		generation: 1,
		msg:        agent.ResponseMsg{Resp: &provider.ChatResponse{Content: "stale"}},
	})
	got := gotModel.(Model)

	if len(got.thread) != 0 {
		t.Fatalf("expected no error thread entry, got %#v", got.thread)
	}
	if got.activeGeneration != 2 {
		t.Fatalf("expected active generation to remain unchanged, got %d", got.activeGeneration)
	}
}

func TestHandleResponseMsgIgnoresCanceledActiveRequest(t *testing.T) {
	m := Model{
		state:            stateThinking,
		activeGeneration: 3,
		vp:               viewport.New(80, 10),
	}

	gotModel, _ := m.handleGenerationMsg(generationMsg{
		generation: 3,
		msg:        agent.ResponseMsg{Err: context.Canceled},
	})
	got := gotModel.(Model)

	if got.activeGeneration != 0 {
		t.Fatalf("expected active generation to be cleared, got %d", got.activeGeneration)
	}
	if len(got.thread) != 0 {
		t.Fatalf("expected no thread entry, got %#v", got.thread)
	}
}

func TestHandleToolExecutedMsgIgnoresStaleGeneration(t *testing.T) {
	m := Model{
		activeGeneration: 2,
		vp:               viewport.New(80, 10),
	}

	gotModel, _ := m.handleGenerationMsg(generationMsg{
		generation: 1,
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

func TestHandleInitialPromptSubmitsPrompt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	suggestionCtx, suggestionCancel := context.WithCancel(context.Background())
	defer suggestionCancel()

	m := Model{
		ctx:              ctx,
		cancel:           cancel,
		suggestionCtx:    suggestionCtx,
		suggestionCancel: suggestionCancel,
		state:            stateIdle,
		vp:               viewport.New(80, 10),
		initialPrompt:    "inspect the repo",
		contextWindow:    32_000,
	}

	gotModel, _ := m.handleInitialPrompt("inspect the repo")
	got := gotModel.(Model)

	if got.state != stateThinking {
		t.Fatalf("expected stateThinking after initial prompt, got %v", got.state)
	}
	if got.initialPrompt != "" {
		t.Fatalf("expected initialPrompt to be cleared, got %q", got.initialPrompt)
	}
	if len(got.thread) != 1 {
		t.Fatalf("expected one thread entry, got %d", len(got.thread))
	}
	if entry, ok := got.thread[0].(UserEntry); !ok || entry.Content != "inspect the repo" {
		t.Fatalf("expected user entry with prefill, got %#v", got.thread[0])
	}
	if len(got.messages) != 1 || got.messages[0].Content != "inspect the repo" {
		t.Fatalf("expected one user message, got %#v", got.messages)
	}
}

func TestHandleIdleKeyTabAcceptsSuggestion(t *testing.T) {
	m := Model{
		state:      stateIdle,
		suggestion: "run the tests",
		input:      textinput.New(),
		vp:         viewport.New(80, 10),
	}

	gotModel, _ := m.handleIdleKey(tea.KeyMsg{Type: tea.KeyTab})
	got := gotModel.(Model)

	if got.input.Value() != "run the tests" {
		t.Fatalf("expected suggestion to be inserted, got %q", got.input.Value())
	}
	if got.suggestion != "" {
		t.Fatalf("expected suggestion to be cleared after accept, got %q", got.suggestion)
	}
}

func TestBuildSuggestionPromptIncludesOriginalRequestAndCommand(t *testing.T) {
	prompt := buildSuggestionPrompt([]ThreadEntry{
		UserEntry{Content: "fix the flaky test"},
		AssistantEntry{Content: "I found the race and updated the test."},
		CommandEntry{Content: "go test ./..."},
	})

	if !strings.Contains(prompt, "Original user request:\nfix the flaky test") {
		t.Fatalf("expected original request in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "Assistant: $ go test ./...") {
		t.Fatalf("expected command entry in prompt, got %q", prompt)
	}
}

func TestShouldFilterSuggestion(t *testing.T) {
	tests := []struct {
		suggestion string
		filtered   bool
	}{
		{suggestion: "run the tests", filtered: false},
		{suggestion: "Looks good", filtered: true},
		{suggestion: "What about logs?", filtered: true},
		{suggestion: "yes", filtered: false},
	}

	for _, tt := range tests {
		if got := shouldFilterSuggestion(tt.suggestion); got != tt.filtered {
			t.Fatalf("shouldFilterSuggestion(%q) = %v, want %v", tt.suggestion, got, tt.filtered)
		}
	}
}

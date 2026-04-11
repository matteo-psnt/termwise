package tui

import (
	"strings"
	"testing"

	"github.com/matteo-psnt/termwise/internal/ai"
)

func TestTrimContextDropsOldToolResultsFirst(t *testing.T) {
	large := strings.Repeat("x", 2_000)
	messages := []ai.Message{
		{
			Role: "user",
			ToolResults: []ai.ToolResult{
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
	messages := []ai.Message{
		{Role: "user", Content: "short"},
		{Role: "assistant", Content: "also short"},
	}

	trimmed := trimContext(messages, 10_000)
	if trimmed[0].Content != "short" || trimmed[1].Content != "also short" {
		t.Fatalf("expected messages to remain unchanged, got %#v", trimmed)
	}
}

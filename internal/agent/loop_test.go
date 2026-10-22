package agent

import (
	"context"
	"testing"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// A command call ends the turn, but calls after it still need results or the
// next request carries tool_use blocks without tool_result, which the APIs reject.
func TestNextToolStepRespondCoversLaterCalls(t *testing.T) {
	calls := []provider.ToolCall{
		readCall("1"),
		{ID: "2", Name: "command", Input: map[string]any{"content": "ls"}},
		readCall("3"),
		{ID: "4", Name: "bash", Input: map[string]any{"command": "echo hi"}},
	}
	collected := []provider.ToolResult{{ToolCallID: "1", Content: "x"}}
	step := NextToolStep(context.Background(), calls[1:], collected, func(string) bool { return true })
	if step.Kind != ToolStepRespond {
		t.Fatalf("kind = %v, want respond", step.Kind)
	}
	if len(step.Collected) != len(calls) {
		t.Fatalf("got %d results for %d calls: %+v", len(step.Collected), len(calls), step.Collected)
	}
	for i, tc := range calls {
		if step.Collected[i].ToolCallID != tc.ID {
			t.Errorf("result %d is for %q, want %q", i, step.Collected[i].ToolCallID, tc.ID)
		}
	}
	for _, r := range step.Collected[2:] {
		if !r.IsError || r.Content != notRunAfterCommand {
			t.Errorf("later call result = %+v, want not-run error", r)
		}
	}
}

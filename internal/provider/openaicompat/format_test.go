package openaicompat

import (
	"testing"

	"github.com/matteo-psnt/termwise/internal/provider"
)

func TestToolResultsPrecedeUserText(t *testing.T) {
	got := toUserMessageParams(provider.Message{
		Role:        "user",
		Content:     "and also",
		ToolResults: []provider.ToolResult{{ToolCallID: "c1", Content: "out"}},
	})
	if len(got) != 2 || got[0].OfTool == nil || got[1].OfUser == nil {
		t.Fatalf("got %+v, want the tool message then the user message", got)
	}
}

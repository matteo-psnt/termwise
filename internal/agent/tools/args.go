package tools

import (
	"context"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// stringInput returns input[key] as a string, or "" when it is absent or not a
// string. Tool arguments arrive as decoded JSON, so a wrong type is treated the
// same as a missing argument.
func stringInput(input map[string]any, key string) string {
	s, _ := input[key].(string)
	return s
}

// intInput returns input[key] as an int. JSON numbers decode as float64, so
// that is the usual case; int is accepted for callers that build input in Go.
// ok=false when the key is absent or not a number.
func intInput(input map[string]any, key string) (int, bool) {
	switch v := input[key].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	}
	return 0, false
}

// executor adapts a tool's (content, isError) implementation to the
// Step.Execute signature, tagging the result with the tool call's ID.
func executor(fn func(context.Context, map[string]any) (string, bool)) func(context.Context, provider.ToolCall) provider.ToolResult {
	return func(ctx context.Context, tc provider.ToolCall) provider.ToolResult {
		content, isErr := fn(ctx, tc.Input)
		return provider.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isErr}
	}
}

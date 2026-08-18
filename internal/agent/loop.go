package agent

import (
	"context"
	"fmt"

	"github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// NextToolStep processes pending tool calls until the next externally-visible step.
// Auto-executed tools (read and safe bash) are executed immediately and returned as
// ToolStepExecuted so callers can decide whether to show progress or continue looping.
func NextToolStep(ctx context.Context, toolCalls []provider.ToolCall, collected []provider.ToolResult, needsApproval func(string) bool) ToolStep {
	for i, tc := range toolCalls {
		switch tc.Name {
		case "command":
			content, _ := tc.Input["content"].(string)
			return ToolStep{
				Kind:      ToolStepRespond,
				ToolCall:  tc,
				Content:   content,
				Collected: append(collected, provider.ToolResult{ToolCallID: tc.ID, Content: "ok"}),
			}

		case "read":
			result := ExecuteRead(tc)
			return ToolStep{
				Kind:         ToolStepExecuted,
				ToolCall:     tc,
				Result:       result,
				Remaining:    toolCalls[i+1:],
				Collected:    append(collected, result),
				AutoAccepted: true,
			}

		case "bash":
			cmd, _ := tc.Input["command"].(string)
			if needsApproval(cmd) {
				return ToolStep{
					Kind:      ToolStepNeedsApproval,
					ToolCall:  tc,
					Command:   cmd,
					Remaining: toolCalls[i+1:],
					Collected: collected,
				}
			}
			result := ExecuteBash(ctx, tc)
			return ToolStep{
				Kind:         ToolStepExecuted,
				ToolCall:     tc,
				Result:       result,
				Remaining:    toolCalls[i+1:],
				Collected:    append(collected, result),
				AutoAccepted: true,
			}

		case "ask":
			question, _ := tc.Input["question"].(string)
			var options []string
			if opts, ok := tc.Input["options"].([]any); ok {
				for _, o := range opts {
					if s, ok := o.(string); ok {
						options = append(options, s)
					}
				}
			}
			multiSelect, _ := tc.Input["multi_select"].(bool)
			return ToolStep{
				Kind:        ToolStepAsk,
				ToolCall:    tc,
				Question:    question,
				Options:     options,
				MultiSelect: multiSelect,
				Remaining:   toolCalls[i+1:],
				Collected:   collected,
			}

		default:
			// Unknown tool — record error and continue.
			collected = append(collected, provider.ToolResult{
				ToolCallID: tc.ID,
				Content:    fmt.Sprintf("error: unknown tool %q", tc.Name),
				IsError:    true,
			})
		}
	}

	return ToolStep{Kind: ToolStepDone, Collected: collected}
}

// ExecuteRead executes a read tool call and returns the tool result.
func ExecuteRead(tc provider.ToolCall) provider.ToolResult {
	content, isErr := tools.Read(tc.Input)
	return provider.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isErr}
}

// ExecuteBash executes a bash tool call and returns the tool result.
func ExecuteBash(ctx context.Context, tc provider.ToolCall) provider.ToolResult {
	content, isErr := tools.Bash(ctx, tc.Input)
	return provider.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isErr}
}

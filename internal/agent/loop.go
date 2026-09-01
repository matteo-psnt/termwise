package agent

import (
	"context"
	"fmt"

	"github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// NextToolStep processes pending tool calls until the next externally-visible
// step. Auto-executed tools run immediately; gated tools (bash) pause for
// approval; respond/ask tools surface intent to the caller.
//
// Dispatch is driven by each tool's registered Step in the tools package — this
// function only knows about Step kinds, not individual tool names.
func NextToolStep(ctx context.Context, toolCalls []provider.ToolCall, collected []provider.ToolResult, needsApproval func(string) bool) ToolStep {
	for i, tc := range toolCalls {
		step, ok := tools.StepFor(tc)
		if !ok {
			collected = append(collected, provider.ToolResult{
				ToolCallID: tc.ID,
				Content:    fmt.Sprintf("error: unknown tool %q", tc.Name),
				IsError:    true,
			})
			continue
		}

		switch step.Kind {
		case tools.StepRespond:
			return ToolStep{
				Kind:      ToolStepRespond,
				ToolCall:  tc,
				Content:   step.Content,
				Collected: append(collected, provider.ToolResult{ToolCallID: tc.ID, Content: "ok"}),
			}

		case tools.StepAsk:
			return ToolStep{
				Kind:        ToolStepAsk,
				ToolCall:    tc,
				Question:    step.Question,
				Options:     step.Options,
				MultiSelect: step.MultiSelect,
				Remaining:   toolCalls[i+1:],
				Collected:   collected,
			}

		case tools.StepGatedExecutor:
			if needsApproval(step.GateCommand) {
				return ToolStep{
					Kind:      ToolStepNeedsApproval,
					ToolCall:  tc,
					Command:   step.GateCommand,
					Remaining: toolCalls[i+1:],
					Collected: collected,
				}
			}
			result := step.Execute(ctx, tc)
			return ToolStep{
				Kind:         ToolStepExecuted,
				ToolCall:     tc,
				Result:       result,
				Remaining:    toolCalls[i+1:],
				Collected:    append(collected, result),
				AutoAccepted: true,
			}

		case tools.StepExecutor:
			result := step.Execute(ctx, tc)
			return ToolStep{
				Kind:         ToolStepExecuted,
				ToolCall:     tc,
				Result:       result,
				Remaining:    toolCalls[i+1:],
				Collected:    append(collected, result),
				AutoAccepted: true,
			}
		}
	}

	return ToolStep{Kind: ToolStepDone, Collected: collected}
}

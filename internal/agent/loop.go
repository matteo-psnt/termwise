package agent

import (
	"context"
	"fmt"

	"github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// notRunAfterCommand is the result for tool calls that follow a command call in
// the same turn: the command ends the turn, so they are never executed.
const notRunAfterCommand = "not run: a command was proposed first"

// NextToolStep processes pending tool calls until the next externally-visible
// step. Auto-executed tools run immediately; gated tools (bash) run immediately
// too unless needsApproval rejects the command, in which case they pause for
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
			collected = append(collected, provider.ToolResult{ToolCallID: tc.ID, Content: "ok"})
			// The turn ends here, but both APIs reject a tool_use without a
			// matching tool_result, so every call after the command still
			// needs one or the next request in this session fails.
			for _, rest := range toolCalls[i+1:] {
				collected = append(collected, provider.ToolResult{
					ToolCallID: rest.ID,
					Content:    notRunAfterCommand,
					IsError:    true,
				})
			}
			return ToolStep{
				Kind:      ToolStepRespond,
				ToolCall:  tc,
				Content:   step.Content,
				Collected: collected,
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
			return executeStep(ctx, step, tc, toolCalls[i+1:], collected)

		case tools.StepExecutor:
			return executeStep(ctx, step, tc, toolCalls[i+1:], collected)
		}
	}

	return ToolStep{Kind: ToolStepDone, Collected: collected}
}

// executeStep runs a step that needs no approval, either because its tool is
// never gated or because the gate let this command through, and reports it as
// auto-accepted.
func executeStep(ctx context.Context, step tools.Step, tc provider.ToolCall, remaining []provider.ToolCall, collected []provider.ToolResult) ToolStep {
	result := step.Execute(ctx, tc)
	return ToolStep{
		Kind:         ToolStepExecuted,
		ToolCall:     tc,
		Result:       result,
		Remaining:    remaining,
		Collected:    append(collected, result),
		AutoAccepted: true,
	}
}

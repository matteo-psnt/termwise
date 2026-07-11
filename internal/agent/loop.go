package agent

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// ChatCmd sends the current conversation to the model and returns a ResponseMsg.
func ChatCmd(ctx context.Context, provider provider.AgentClient, req provider.ChatRequest) tea.Cmd {
	return func() tea.Msg {
		resp, err := provider.Chat(ctx, req)
		return ResponseMsg{Resp: resp, Err: err}
	}
}

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

// ProcessToolsCmd processes pending tool calls in order, returning on the first
// one that requires user input (approval or ask). Auto-executed tools (read)
// return immediately so the TUI can update the display and continue.
// needsApproval is called for bash commands to determine if approval is required.
func ProcessToolsCmd(ctx context.Context, toolCalls []provider.ToolCall, collected []provider.ToolResult, needsApproval func(string) bool) tea.Cmd {
	return func() tea.Msg {
		step := NextToolStep(ctx, toolCalls, collected, needsApproval)
		switch step.Kind {
		case ToolStepRespond:
			return RespondMsg{
				ToolCallID: step.ToolCall.ID,
				Content:    step.Content,
				Collected:  step.Collected,
			}
		case ToolStepExecuted:
			return ToolExecutedMsg{
				ToolCall:     step.ToolCall,
				Result:       step.Result,
				Remaining:    step.Remaining,
				Collected:    step.Collected,
				AutoAccepted: step.AutoAccepted,
			}
		case ToolStepNeedsApproval:
			return NeedsApprovalMsg{
				ToolCall:  step.ToolCall,
				Command:   step.Command,
				Remaining: step.Remaining,
				Collected: step.Collected,
			}
		case ToolStepAsk:
			return AskMsg{
				ToolCall:    step.ToolCall,
				Question:    step.Question,
				Options:     step.Options,
				MultiSelect: step.MultiSelect,
				Remaining:   step.Remaining,
				Collected:   step.Collected,
			}
		default:
			return AllToolsDoneMsg{Collected: step.Collected}
		}
	}
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

// ExecuteBashCmd executes a bash command after the user has approved it.
func ExecuteBashCmd(ctx context.Context, tc provider.ToolCall, remaining []provider.ToolCall, collected []provider.ToolResult) tea.Cmd {
	return func() tea.Msg {
		result := ExecuteBash(ctx, tc)
		return ToolExecutedMsg{
			ToolCall:     tc,
			Result:       result,
			Remaining:    remaining,
			Collected:    append(collected, result),
			AutoAccepted: false,
		}
	}
}

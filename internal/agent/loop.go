package agent

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteo-psnt/termwise/internal/ai"
	"github.com/matteo-psnt/termwise/internal/tools"
)

// ChatCmd sends the current conversation to the model and returns a ResponseMsg.
func ChatCmd(ctx context.Context, provider ai.AgentProvider, req ai.ChatRequest) tea.Cmd {
	return func() tea.Msg {
		resp, err := provider.Chat(ctx, req)
		return ResponseMsg{Resp: resp, Err: err}
	}
}

// ProcessToolsCmd processes pending tool calls in order, returning on the first
// one that requires user input (approval or ask). Auto-executed tools (read)
// return immediately so the TUI can update the display and continue.
// needsApproval is called for bash commands to determine if approval is required.
func ProcessToolsCmd(toolCalls []ai.ToolCall, collected []ai.ToolResult, needsApproval func(string) bool) tea.Cmd {
	return func() tea.Msg {
		for i, tc := range toolCalls {
			remaining := make([]ai.ToolCall, len(toolCalls)-i-1)
			copy(remaining, toolCalls[i+1:])

			switch tc.Name {
			case "respond":
				respondType, _ := tc.Input["type"].(string)
				content, _ := tc.Input["content"].(string)
				if respondType == "" {
					respondType = "text"
				}
				return RespondMsg{
					ToolCallID:  tc.ID,
					RespondType: respondType,
					Content:     content,
					Collected:   append(collected, ai.ToolResult{ToolCallID: tc.ID, Content: "ok"}),
				}

			case "read":
				content, isErr := tools.Read(tc.Input)
				result := ai.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isErr}
				return ToolExecutedMsg{
					ToolCall:     tc,
					Result:       result,
					Remaining:    remaining,
					Collected:    append(collected, result),
					AutoAccepted: true,
				}

			case "bash":
				cmd, _ := tc.Input["command"].(string)
				if needsApproval(cmd) {
					return NeedsApprovalMsg{
						ToolCall:  tc,
						Command:   cmd,
						Remaining: remaining,
						Collected: collected,
					}
				}
				content, isErr := tools.Bash(tc.Input)
				result := ai.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isErr}
				return ToolExecutedMsg{
					ToolCall:     tc,
					Result:       result,
					Remaining:    remaining,
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
				return AskMsg{
					ToolCall:    tc,
					Question:    question,
					Options:     options,
					MultiSelect: multiSelect,
					Remaining:   remaining,
					Collected:   collected,
				}

			default:
				// Unknown tool — record error and continue.
				result := ai.ToolResult{
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("error: unknown tool %q", tc.Name),
					IsError:    true,
				}
				collected = append(collected, result)
			}
		}
		return AllToolsDoneMsg{Collected: collected}
	}
}

// ExecuteBashCmd executes a bash command after the user has approved it.
func ExecuteBashCmd(tc ai.ToolCall, remaining []ai.ToolCall, collected []ai.ToolResult) tea.Cmd {
	return func() tea.Msg {
		content, isErr := tools.Bash(tc.Input)
		result := ai.ToolResult{ToolCallID: tc.ID, Content: content, IsError: isErr}
		return ToolExecutedMsg{
			ToolCall:     tc,
			Result:       result,
			Remaining:    remaining,
			Collected:    append(collected, result),
			AutoAccepted: false,
		}
	}
}

package agentui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/allowlist"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// Events the agent commands below deliver to Update. Each carries what the
// matching handler in events.go needs to continue the turn.

// responseEvent reports a chat response from the model.
type responseEvent struct {
	Resp *provider.ChatResponse
	Err  error
}

// toolExecutedEvent reports that a tool ran without needing approval.
type toolExecutedEvent struct {
	ToolCall     provider.ToolCall
	Result       provider.ToolResult
	Remaining    []provider.ToolCall
	Collected    []provider.ToolResult
	AutoAccepted bool
}

// needsApprovalEvent reports that a bash command requires user approval before running.
type needsApprovalEvent struct {
	ToolCall  provider.ToolCall
	Command   string
	Remaining []provider.ToolCall
	Collected []provider.ToolResult
}

// askEvent reports that the model used the ask tool.
type askEvent struct {
	ToolCall    provider.ToolCall
	Question    string
	Options     []string
	MultiSelect bool
	Remaining   []provider.ToolCall
	Collected   []provider.ToolResult
}

// respondEvent reports that the model called the command tool.
type respondEvent struct {
	Content   string
	Collected []provider.ToolResult
}

// allToolsDoneEvent reports that all tool calls have been processed and results
// should be sent back to the model.
type allToolsDoneEvent struct {
	Collected []provider.ToolResult
}

// chatCmd sends the current conversation to the model and returns a responseEvent.
func chatCmd(ctx context.Context, client provider.AgentClient, req provider.ChatRequest) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.Chat(ctx, req)
		return responseEvent{Resp: resp, Err: err}
	}
}

// processToolsCmd processes pending tool calls in order, returning on the first
// one that requires user input (approval or ask). Auto-executed tools (read)
// return immediately so the TUI can update the display and continue. Bash
// commands the allowlist does not clear stop for approval.
func processToolsCmd(ctx context.Context, toolCalls []provider.ToolCall, collected []provider.ToolResult) tea.Cmd {
	return func() tea.Msg {
		step := agent.NextToolStep(ctx, toolCalls, collected, allowlist.NeedsApproval)
		switch step.Kind {
		case agent.ToolStepRespond:
			return respondEvent{
				Content:   step.Content,
				Collected: step.Collected,
			}
		case agent.ToolStepExecuted:
			return toolExecutedEvent{
				ToolCall:     step.ToolCall,
				Result:       step.Result,
				Remaining:    step.Remaining,
				Collected:    step.Collected,
				AutoAccepted: step.AutoAccepted,
			}
		case agent.ToolStepNeedsApproval:
			return needsApprovalEvent{
				ToolCall:  step.ToolCall,
				Command:   step.Command,
				Remaining: step.Remaining,
				Collected: step.Collected,
			}
		case agent.ToolStepAsk:
			return askEvent{
				ToolCall:    step.ToolCall,
				Question:    step.Question,
				Options:     step.Options,
				MultiSelect: step.MultiSelect,
				Remaining:   step.Remaining,
				Collected:   step.Collected,
			}
		default:
			return allToolsDoneEvent{Collected: step.Collected}
		}
	}
}

// executeBashCmd executes a bash command after the user has approved it.
func executeBashCmd(ctx context.Context, tc provider.ToolCall, remaining []provider.ToolCall, collected []provider.ToolResult) tea.Cmd {
	return func() tea.Msg {
		result := tools.ExecuteBash(ctx, tc)
		return toolExecutedEvent{
			ToolCall:     tc,
			Result:       result,
			Remaining:    remaining,
			Collected:    append(collected, result),
			AutoAccepted: false,
		}
	}
}

type judgmentMsg struct {
	safe bool
}

func (m Model) judgeCmd(tc provider.ToolCall) tea.Cmd {
	cmd := tools.BashCommand(tc)
	client := m.provider
	modelID := m.modelID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		safe, err := agent.JudgeBashCommand(ctx, client, modelID, cmd)
		if err != nil {
			return judgmentMsg{safe: false}
		}
		return judgmentMsg{safe: safe}
	}
}

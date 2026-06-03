package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// FinalResponse is the terminal response produced by a headless agent run.
type FinalResponse struct {
	Type    string
	Content string
}

// HeadlessConfig configures a non-interactive agent run.
type HeadlessConfig struct {
	Client          provider.AgentClient
	Model           string
	System          string
	Prompt          string
	Tools           []provider.ToolDef
	NeedsApproval   func(string) bool
	OnNeedsApproval func(context.Context, ToolStep) provider.ToolResult
	OnAsk           func(ToolStep) provider.ToolResult
}

// RunHeadless runs the agent loop without UI and returns only the final response.
func RunHeadless(ctx context.Context, cfg HeadlessConfig) (FinalResponse, error) {
	needsApproval := cfg.NeedsApproval
	if needsApproval == nil {
		needsApproval = func(string) bool { return false }
	}
	onNeedsApproval := cfg.OnNeedsApproval
	if onNeedsApproval == nil {
		onNeedsApproval = func(_ context.Context, step ToolStep) provider.ToolResult {
			return provider.ToolResult{
				ToolCallID: step.ToolCall.ID,
				Content:    "error: no approval handler configured",
				IsError:    true,
			}
		}
	}
	onAsk := cfg.OnAsk
	if onAsk == nil {
		onAsk = func(step ToolStep) provider.ToolResult {
			return provider.ToolResult{
				ToolCallID: step.ToolCall.ID,
				Content:    "error: ask tool is unavailable in headless mode",
				IsError:    true,
			}
		}
	}

	messages := []provider.Message{{Role: "user", Content: cfg.Prompt}}

	for {
		resp, err := cfg.Client.Chat(ctx, provider.ChatRequest{
			Model:    cfg.Model,
			System:   cfg.System,
			Messages: messages,
			Tools:    cfg.Tools,
		})
		if err != nil {
			return FinalResponse{}, err
		}

		if len(resp.ToolCalls) == 0 {
			output := FinalResponse{Type: "text", Content: strings.TrimSpace(resp.Content)}
			if output.Content == "" {
				return FinalResponse{}, fmt.Errorf("agent returned no final response")
			}
			return output, nil
		}

		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		collected := []provider.ToolResult{}
		remaining := resp.ToolCalls
		for {
			step := NextToolStep(ctx, remaining, collected, needsApproval)

			switch step.Kind {
			case ToolStepRespond:
				return FinalResponse{
					Type:    step.RespondType,
					Content: strings.TrimSpace(step.Content),
				}, nil

			case ToolStepExecuted:
				collected = step.Collected
				remaining = step.Remaining

			case ToolStepNeedsApproval:
				collected = append(step.Collected, onNeedsApproval(ctx, step))
				remaining = step.Remaining

			case ToolStepAsk:
				collected = append(step.Collected, onAsk(step))
				remaining = step.Remaining

			case ToolStepDone:
				messages = append(messages, provider.Message{
					Role:        "user",
					ToolResults: step.Collected,
				})
				goto nextTurn

			default:
				return FinalResponse{}, fmt.Errorf("unknown tool step kind %d", step.Kind)
			}
		}

	nextTurn:
	}
}

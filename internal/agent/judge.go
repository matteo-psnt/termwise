package agent

import (
	"context"
	"strings"

	"github.com/matteo-psnt/termwise/internal/provider"
)

const judgeSystemPrompt = "You are a safety classifier for bash commands. Respond with exactly one word: safe or unsafe."

// JudgeBashCommand asks the configured model whether a bash command is safe to
// auto-execute without interactive confirmation.
func JudgeBashCommand(ctx context.Context, client provider.AgentClient, modelID string, command string) (bool, error) {
	resp, err := client.Chat(ctx, provider.ChatRequest{
		Model:  modelID,
		System: judgeSystemPrompt,
		Messages: []provider.Message{
			{Role: "user", Content: "Should this bash command auto-execute without user confirmation?\n\n" + command},
		},
	})
	if err != nil {
		return false, err
	}
	return strings.ToLower(strings.TrimSpace(resp.Content)) == "safe", nil
}

package agentui

import (
	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// logExchange records a completed turn — the prompt that started it and the
// answer it produced — so output quality can be reviewed later. Failures are
// swallowed: a logging problem must never disturb a session.
func (m *Model) logExchange(kind, response string) {
	prompt, turns, tools := m.currentTurn()
	_ = m.exchanges.Append(history.Exchange{
		Prompt:   prompt,
		Kind:     kind,
		Response: response,
		Provider: m.providerName,
		Model:    m.modelID,
		Tools:    tools,
		Turns:    turns,
	})
}

// currentTurn walks back to the user message that began the current turn and
// reports it along with the model round-trips and tool calls spent since.
func (m Model) currentTurn() (prompt string, turns int, tools []string) {
	start := -1
	for i := len(m.messages) - 1; i >= 0; i-- {
		if isUserPrompt(m.messages[i]) {
			start = i
			break
		}
	}
	if start < 0 {
		return "", 0, nil
	}
	prompt = m.messages[start].Content
	for _, msg := range m.messages[start+1:] {
		if msg.Role != "assistant" {
			continue
		}
		turns++
		for _, tc := range msg.ToolCalls {
			tools = append(tools, tc.Name)
		}
	}
	return prompt, turns, tools
}

// isUserPrompt distinguishes a typed prompt from the synthetic user messages
// that carry tool results back to the model.
func isUserPrompt(msg provider.Message) bool {
	return msg.Role == "user" && msg.Content != "" && len(msg.ToolResults) == 0
}

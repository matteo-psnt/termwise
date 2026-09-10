package agentui

import (
	"testing"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// currentTurn must find the user's typed prompt, not the synthetic user
// messages that carry tool results back to the model.
func TestCurrentTurnSkipsToolResultMessages(t *testing.T) {
	m := Model{}
	m.messages = []provider.Message{
		{Role: "user", Content: "earlier question"},
		{Role: "assistant", Content: "earlier answer"},
		{Role: "user", Content: "install gemini"},
		{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "1", Name: "bash"}}},
		{Role: "user", ToolResults: []provider.ToolResult{{ToolCallID: "1", Content: "brew ok"}}},
		{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "2", Name: "command"}}},
	}

	prompt, turns, tools := m.currentTurn()
	if prompt != "install gemini" {
		t.Errorf("prompt = %q, want %q", prompt, "install gemini")
	}
	if turns != 2 {
		t.Errorf("turns = %d, want 2", turns)
	}
	if len(tools) != 2 || tools[0] != "bash" || tools[1] != "command" {
		t.Errorf("tools = %v, want [bash command]", tools)
	}
}

func TestCurrentTurnCountsASingleTurn(t *testing.T) {
	m := Model{}
	m.messages = []provider.Message{
		{Role: "user", Content: "what is this project"},
		{Role: "assistant", Content: "a terminal assistant"},
	}
	prompt, turns, tools := m.currentTurn()
	if prompt != "what is this project" || turns != 1 || len(tools) != 0 {
		t.Errorf("currentTurn() = (%q, %d, %v)", prompt, turns, tools)
	}
}

func TestCurrentTurnWithNoUserMessage(t *testing.T) {
	m := Model{}
	prompt, turns, tools := m.currentTurn()
	if prompt != "" || turns != 0 || tools != nil {
		t.Errorf("currentTurn() on empty history = (%q, %d, %v)", prompt, turns, tools)
	}
}

// A nil exchange log must be a silent no-op — the TUI runs without one when no
// config directory could be resolved.
func TestLogExchangeWithNilLogDoesNotPanic(_ *testing.T) {
	m := Model{}
	m.messages = []provider.Message{{Role: "user", Content: "hi"}}
	m.logExchange("text", "hello")
}

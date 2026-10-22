package provider

import (
	"context"
	"encoding/json"
	"time"
)

// ----------------------------------------------------------------------------
// Shared types
// ----------------------------------------------------------------------------

// Model describes a model available from a provider.
type Model struct {
	ID          string // API identifier, e.g. "claude-haiku-4-5-20251001"
	DisplayName string // human-readable, e.g. "Claude Haiku 4.5"
}

// ----------------------------------------------------------------------------
// Single-shot types
// ----------------------------------------------------------------------------

// CompleteRequest is the input to Client.Complete.
type CompleteRequest struct {
	Model  string // model ID
	System string // system prompt
	Prompt string // user prompt (query + stdin already combined by caller)
}

// CompleteResponse is the output of Provider.Complete.
type CompleteResponse struct {
	Content      string // raw model response text
	InputTokens  int
	OutputTokens int
}

// ----------------------------------------------------------------------------
// Agent types
// ----------------------------------------------------------------------------

// Message is one turn in a multi-turn conversation.
type Message struct {
	Role        string       // "user" or "assistant"
	Content     string       // text content (for user messages or bare assistant text)
	ToolCalls   []ToolCall   // tool calls made by the assistant in this turn
	ToolResults []ToolResult // tool results sent by the user in this turn
}

// ToolCall is a single tool invocation requested by the model.
type ToolCall struct {
	ID    string         // provider-assigned call ID
	Name  string         // tool name: "bash", "read", "ask", "respond"
	Input map[string]any // parsed JSON input
}

// ToolResult is the result of executing a tool call.
type ToolResult struct {
	ToolCallID string
	Content    string
	IsError    bool
}

// ToolDef describes a tool the model can call.
type ToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any // JSON Schema object
}

// ToolNames returns the tool names in order.
func ToolNames(defs []ToolDef) []string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}

// ChatRequest is the input to AgentClient.Chat.
type ChatRequest struct {
	Model    string
	System   string
	Messages []Message
	Tools    []ToolDef
	// Effort sets the reasoning/thinking effort level ("low", "medium", "high").
	// Empty means no reasoning config is sent (provider default).
	Effort string
}

// ChatResponse is the output of AgentClient.Chat.
type ChatResponse struct {
	Content      string // bare text if model responded without tool calls
	ToolCalls    []ToolCall
	StopReason   string // "end_turn" or "tool_use"
	InputTokens  int
	OutputTokens int
}

// ----------------------------------------------------------------------------
// Interfaces
// ----------------------------------------------------------------------------

// Client is the minimal interface for single-shot mode.
type Client interface {
	// Name returns the provider identifier (e.g. "anthropic").
	Name() string

	// ListModels returns the models available from this provider.
	ListModels(ctx context.Context) ([]Model, error)

	// Complete sends a single prompt and returns the response.
	Complete(ctx context.Context, req CompleteRequest) (*CompleteResponse, error)
}

// AgentClient extends Client with multi-turn conversation and tool use.
// Used for agent mode (TUI). Both the Anthropic and OpenAI-compatible clients
// implement this interface.
type AgentClient interface {
	Client
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

// Config is the resolved config passed to provider constructors.
// Auth is already resolved — no keychain/env lookups happen inside providers.
type Config struct {
	APIKey  string // resolved API key (empty for providers that need no auth, e.g. ollama)
	Model   string // model ID to use
	BaseURL string // optional override; empty means use provider default
}

// ----------------------------------------------------------------------------
// Helpers for client packages
// ----------------------------------------------------------------------------

// DefaultRequestTimeout bounds each HTTP request a client makes.
const DefaultRequestTimeout = 30 * time.Second

// ParseToolInput unmarshals a tool call's raw JSON arguments into a map.
// Empty input yields an empty map, since models may omit arguments for a
// tool that takes none.
func ParseToolInput(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

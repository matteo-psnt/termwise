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
// Agent types
// ----------------------------------------------------------------------------

// Message is one turn in a multi-turn conversation.
type Message struct {
	Role        string       // "user" or "assistant"
	Content     string       // text content (for user messages or bare assistant text)
	ToolCalls   []ToolCall   // tool calls made by the assistant in this turn
	ToolResults []ToolResult // tool results sent by the user in this turn
	// ProviderState is opaque provider-specific data copied from the
	// ChatResponse that produced this assistant turn, which the same provider
	// needs back verbatim when the conversation continues (Anthropic's signed
	// thinking blocks). Other providers ignore it. omitempty keeps saved
	// sessions without it loading unchanged.
	ProviderState json.RawMessage `json:",omitempty"`
}

// ToolCall is a single tool invocation requested by the model.
type ToolCall struct {
	ID    string         // provider-assigned call ID
	Name  string         // tool name, e.g. "bash", "read", "grep", "glob", "ask", "command"
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
	Content   string // bare text if model responded without tool calls
	ToolCalls []ToolCall
	// StopReason is the provider's raw stop/finish reason, passed through
	// unnormalised: Anthropic reports e.g. "end_turn" or "tool_use", OpenAI-
	// compatible APIs e.g. "stop" or "tool_calls".
	StopReason   string
	InputTokens  int
	OutputTokens int
	// ProviderState must be copied onto the assistant Message built from
	// this response; see Message.ProviderState.
	ProviderState json.RawMessage
	// HistoryStateDropped reports that the provider rejected the
	// ProviderState replayed in the request's history and answered without
	// it. The caller must drop it from its own history too (WithoutState),
	// or every later request will be rejected and retried the same way.
	HistoryStateDropped bool
}

// WithoutState returns a copy of msgs with every ProviderState removed;
// text, tool calls and tool results are kept.
func WithoutState(msgs []Message) []Message {
	out := make([]Message, len(msgs))
	for i, m := range msgs {
		m.ProviderState = nil
		out[i] = m
	}
	return out
}

// ----------------------------------------------------------------------------
// Interfaces
// ----------------------------------------------------------------------------

// AgentClient is what every provider implements: the TUI, the headless path
// and one-shot uses such as input suggestions all go through Chat. A one-shot
// prompt is a ChatRequest with a single user message and no tools.
type AgentClient interface {
	// ListModels returns the models available from this provider.
	ListModels(ctx context.Context) ([]Model, error)

	// Chat sends a conversation, with any tool definitions, and returns the
	// model's next turn.
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

// DefaultRequestTimeout bounds each HTTP request a client makes. Requests are
// not streamed, so it must cover a whole response: a 16k-token thinking turn
// at a slow ~40 tokens/s takes close to 7 minutes, and the Anthropic SDK itself
// expects up to 7.5 minutes for that budget. Ten minutes matches the SDK's own
// non-streaming default. The cap only catches a stalled connection: Esc and
// ctrl+c cancel the request context, which ends it immediately.
const DefaultRequestTimeout = 10 * time.Minute

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

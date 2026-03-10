package ai

import (
	"context"
	"fmt"
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

// CompleteRequest is the input to Provider.Complete.
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

// ChatRequest is the input to AgentProvider.Chat.
type ChatRequest struct {
	Model    string
	System   string
	Messages []Message
	Tools    []ToolDef
}

// ChatResponse is the output of AgentProvider.Chat.
type ChatResponse struct {
	Content      string     // bare text if model responded without tool calls
	ToolCalls    []ToolCall
	StopReason   string // "end_turn" or "tool_use"
	InputTokens  int
	OutputTokens int
}

// ----------------------------------------------------------------------------
// Interfaces
// ----------------------------------------------------------------------------

// Provider is the minimal interface for single-shot mode.
type Provider interface {
	// Name returns the provider identifier (e.g. "anthropic").
	Name() string

	// ListModels returns the models available from this provider.
	ListModels(ctx context.Context) ([]Model, error)

	// Complete sends a single prompt and returns the response.
	Complete(ctx context.Context, req CompleteRequest) (*CompleteResponse, error)
}

// AgentProvider extends Provider with multi-turn conversation and tool use.
// Used for agent mode (TUI). Both the Anthropic and OpenAI-compatible clients
// implement this interface.
type AgentProvider interface {
	Provider
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

// ----------------------------------------------------------------------------
// Registry
// ----------------------------------------------------------------------------

// ProviderConfig is the resolved config passed to provider constructors.
// Auth is already resolved — no keychain/env lookups happen inside providers.
type ProviderConfig struct {
	APIKey  string // resolved API key (empty for providers that need no auth, e.g. ollama)
	Model   string // model ID to use
	BaseURL string // optional override; empty means use provider default
}

// NewProviderFunc is the constructor signature for all providers.
type NewProviderFunc func(cfg ProviderConfig) (AgentProvider, error)

var registry = map[string]NewProviderFunc{}

// Register adds a provider constructor to the registry.
// Called from each provider package's init().
func Register(name string, fn NewProviderFunc) {
	registry[name] = fn
}

// GetProvider looks up and constructs a provider by name.
func GetProvider(name string, cfg ProviderConfig) (AgentProvider, error) {
	fn, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q — supported: %v", name, registeredNames())
	}
	return fn(cfg)
}

func registeredNames() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}

// ----------------------------------------------------------------------------
// Errors
// ----------------------------------------------------------------------------

// ErrorKind categorises provider errors for the CLI/TUI layer.
type ErrorKind int

const (
	ErrAuth ErrorKind = iota
	ErrRateLimit
	ErrTimeout
	ErrModelNotFound
	ErrInputTooLarge
	ErrProvider // catch-all
)

// ProviderError carries a user-facing reason and an underlying technical error.
type ProviderError struct {
	Kind   ErrorKind
	Reason string // shown to the user
	Err    error  // underlying detail for debugging
}

func (e *ProviderError) Error() string { return e.Reason }
func (e *ProviderError) Unwrap() error { return e.Err }

// providerErr is a convenience constructor used by client packages.
func NewProviderErr(kind ErrorKind, reason string, err error) *ProviderError {
	return &ProviderError{Kind: kind, Reason: reason, Err: err}
}

package openaicompat

import (
	"context"
	"errors"
	"fmt"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// Client implements provider.AgentClient for any OpenAI-compatible API.
type Client struct {
	sdk  openai.Client
	name string
}

// New constructs a client for the named provider using its default base URL,
// overridden by cfg.BaseURL if set.
func New(name, defaultBaseURL string, cfg provider.Config) (*Client, error) {
	baseURL := defaultBaseURL
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(baseURL),
		option.WithRequestTimeout(provider.DefaultRequestTimeout),
	}
	return &Client{
		sdk:  openai.NewClient(opts...),
		name: name,
	}, nil
}

// ListModels fetches available models and applies per-provider filtering.
func (c *Client) ListModels(ctx context.Context) ([]provider.Model, error) {
	pager := c.sdk.Models.ListAutoPaging(ctx)
	var models []provider.Model
	for pager.Next() {
		m := pager.Current()
		models = append(models, provider.Model{
			ID:          m.ID,
			DisplayName: m.ID,
		})
	}
	if err := pager.Err(); err != nil {
		return nil, c.mapErr(err)
	}
	return filterModels(c.name, models), nil
}

// Chat sends a multi-turn conversation with tool definitions and returns the response.
func (c *Client) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	msgs, err := toMessageParams(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("building messages: %w", err)
	}
	tools := toToolParams(req.Tools)
	params := toParams(req.Model, req.System, msgs, tools, req.Effort)

	msg, err := c.sdk.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, c.mapErr(err)
	}
	// A refusal arrives as a successful response with empty content, which
	// would otherwise show as a blank answer.
	if len(msg.Choices) > 0 && msg.Choices[0].Message.Refusal != "" {
		return nil, provider.Refusal(c.displayName(), "", msg.Choices[0].Message.Refusal)
	}
	return fromChatResponse(msg)
}

// mapErr converts SDK errors to our provider.Error type.
func (c *Client) mapErr(err error) error {
	return provider.MapError(c.displayName(), err, func(err error) (int, bool) {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			return apiErr.StatusCode, true
		}
		return 0, false
	})
}

// displayName names the provider in user-facing errors.
func (c *Client) displayName() string {
	if info, ok := provider.Lookup(c.name); ok {
		return info.DisplayName
	}
	return c.name
}

package openaicompat

import (
	"context"
	"errors"
	"fmt"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/matteo-psnt/termwise/internal/provider"
)

const requestTimeout = 30 * time.Second

// Client implements provider.AgentClient for any OpenAI-compatible API.
type Client struct {
	sdk      openai.Client
	provider string
	cfg      provider.Config
}

// New constructs a client for the named provider using its default base URL,
// overridden by cfg.BaseURL if set.
func New(provider, defaultBaseURL string, cfg provider.Config) (*Client, error) {
	baseURL := defaultBaseURL
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(baseURL),
		option.WithRequestTimeout(requestTimeout),
	}
	return &Client{
		sdk:      openai.NewClient(opts...),
		provider: provider,
		cfg:      cfg,
	}, nil
}

func (c *Client) Name() string { return c.provider }

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
		return nil, mapErr(err)
	}
	return filterModels(c.provider, models), nil
}

// Complete sends a single-shot prompt and returns the full response.
func (c *Client) Complete(ctx context.Context, req provider.CompleteRequest) (*provider.CompleteResponse, error) {
	msgs := []openai.ChatCompletionMessageParamUnion{openai.UserMessage(req.Prompt)}
	params := toParams(req.Model, req.System, msgs, nil)

	msg, err := c.sdk.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, mapErr(err)
	}
	return fromResponse(msg), nil
}

// Chat sends a multi-turn conversation with tool definitions and returns the response.
func (c *Client) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	msgs, err := toMessageParams(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("building messages: %w", err)
	}
	tools := toToolParams(req.Tools)
	params := toParamsWithEffort(req.Model, req.System, msgs, tools, req.Effort)

	msg, err := c.sdk.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, mapErr(err)
	}
	return fromChatResponse(msg)
}

// mapErr converts SDK errors to our provider.Error type.
func mapErr(err error) error {
	return provider.MapError(err, func(err error) (int, bool) {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			return apiErr.StatusCode, true
		}
		return 0, false
	})
}

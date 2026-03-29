package anthropic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	sdkoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/matteo-psnt/termwise/internal/ai"
)

const requestTimeout = 30 * time.Second

func init() {
	ai.Register("anthropic", func(cfg ai.ProviderConfig) (ai.AgentProvider, error) {
		return New(cfg)
	})
}

// Client implements ai.AgentProvider for the Anthropic API.
type Client struct {
	sdk sdk.Client
	cfg ai.ProviderConfig
}

// New constructs an Anthropic client from a resolved ProviderConfig.
func New(cfg ai.ProviderConfig) (*Client, error) {
	opts := []sdkoption.RequestOption{
		sdkoption.WithAPIKey(cfg.APIKey),
		sdkoption.WithRequestTimeout(requestTimeout),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, sdkoption.WithBaseURL(cfg.BaseURL))
	}
	return &Client{
		sdk: sdk.NewClient(opts...),
		cfg: cfg,
	}, nil
}

func (c *Client) Name() string { return "anthropic" }

// ListModels fetches available models from the Anthropic API.
func (c *Client) ListModels(ctx context.Context) ([]ai.Model, error) {
	page, err := c.sdk.Models.List(ctx, sdk.ModelListParams{})
	if err != nil {
		return nil, mapErr(err)
	}
	var out []ai.Model
	for _, m := range page.Data {
		out = append(out, ai.Model{
			ID:          string(m.ID),
			DisplayName: m.DisplayName,
		})
	}
	return out, nil
}

// Complete sends a single-shot prompt and returns the full response.
func (c *Client) Complete(ctx context.Context, req ai.CompleteRequest) (*ai.CompleteResponse, error) {
	msgs := toSingleMessage(req.Prompt)
	params := toParams(req.Model, req.System, msgs, nil)

	msg, err := c.sdk.Messages.New(ctx, params)
	if err != nil {
		return nil, mapErr(err)
	}
	return fromResponse(msg), nil
}

// Chat sends a multi-turn conversation with tool definitions and returns the response.
func (c *Client) Chat(ctx context.Context, req ai.ChatRequest) (*ai.ChatResponse, error) {
	msgs, err := toMessageParams(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("building messages: %w", err)
	}

	tools := toToolParams(req.Tools)
	params := toParams(req.Model, req.System, msgs, tools)

	msg, err := c.sdk.Messages.New(ctx, params)
	if err != nil {
		return nil, mapErr(err)
	}
	return fromChatResponse(msg)
}

// mapErr converts SDK errors to our ProviderError type.
func mapErr(err error) error {
	if err == nil {
		return nil
	}

	// Context cancellation (ctrl+c).
	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ai.NewProviderErr(ai.ErrTimeout, "request timed out", err)
	}

	// Anthropic SDK wraps HTTP errors as *sdk.Error (alias for *apierror.Error).
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		return mapAPIErr(apiErr)
	}

	return ai.NewProviderErr(ai.ErrProvider, err.Error(), err)
}

func mapAPIErr(err *sdk.Error) *ai.ProviderError {
	body := err.Error()
	switch err.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ai.NewProviderErr(ai.ErrAuth,
			"API key invalid or expired.", err)

	case http.StatusTooManyRequests:
		return ai.NewProviderErr(ai.ErrRateLimit,
			"Rate limited. Try again in a moment.", err)

	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return ai.NewProviderErr(ai.ErrTimeout,
			"Request timed out.", err)

	case http.StatusNotFound:
		return ai.NewProviderErr(ai.ErrModelNotFound,
			"Model not found.", err)

	case http.StatusBadRequest:
		if strings.Contains(body, "too large") || strings.Contains(body, "too long") {
			return ai.NewProviderErr(ai.ErrInputTooLarge,
				"Input too large for this model. Try piping less data.", err)
		}
		return ai.NewProviderErr(ai.ErrProvider, body, err)

	default:
		return ai.NewProviderErr(ai.ErrProvider, body, err)
	}
}

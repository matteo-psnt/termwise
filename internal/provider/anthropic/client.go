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

	"github.com/matteo-psnt/termwise/internal/provider"
)

const requestTimeout = 30 * time.Second

// Client implements provider.AgentClient for the Anthropic API.
type Client struct {
	sdk sdk.Client
	cfg provider.Config
}

// New constructs an Anthropic client from a resolved provider.Config.
func New(cfg provider.Config) (*Client, error) {
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

func (*Client) Name() string { return "anthropic" }

// ListModels fetches available models from the Anthropic API.
func (c *Client) ListModels(ctx context.Context) ([]provider.Model, error) {
	page, err := c.sdk.Models.List(ctx, sdk.ModelListParams{})
	if err != nil {
		return nil, mapErr(err)
	}
	var out []provider.Model
	for _, m := range page.Data {
		out = append(out, provider.Model{
			ID:          m.ID,
			DisplayName: m.DisplayName,
		})
	}
	return out, nil
}

// Complete sends a single-shot prompt and returns the full response.
func (c *Client) Complete(ctx context.Context, req provider.CompleteRequest) (*provider.CompleteResponse, error) {
	msgs := toSingleMessage(req.Prompt)
	params := toParams(req.Model, req.System, msgs, nil)

	msg, err := c.sdk.Messages.New(ctx, params)
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

	msg, err := c.sdk.Messages.New(ctx, params)
	if err != nil {
		return nil, mapErr(err)
	}
	return fromChatResponse(msg)
}

// mapErr converts SDK errors to our provider.Error type.
func mapErr(err error) error {
	if err == nil {
		return nil
	}

	// Context cancellation (ctrl+c).
	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return provider.NewError(provider.ErrTimeout, "request timed out", err)
	}

	// Anthropic SDK wraps HTTP errors as *sdk.Error (alias for *apierror.Error).
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		return mapAPIErr(apiErr)
	}

	return provider.NewError(provider.ErrProvider, err.Error(), err)
}

func mapAPIErr(err *sdk.Error) *provider.Error {
	body := err.Error()
	switch err.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return provider.NewError(provider.ErrAuth,
			"API key invalid or expired.", err)

	case http.StatusTooManyRequests:
		return provider.NewError(provider.ErrRateLimit,
			"Rate limited. Try again in a moment.", err)

	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return provider.NewError(provider.ErrTimeout,
			"Request timed out.", err)

	case http.StatusNotFound:
		return provider.NewError(provider.ErrModelNotFound,
			"Model not found.", err)

	case http.StatusBadRequest:
		if strings.Contains(body, "too large") || strings.Contains(body, "too long") {
			return provider.NewError(provider.ErrInputTooLarge,
				"Input too large for this model. Try piping less data.", err)
		}
		return provider.NewError(provider.ErrProvider, body, err)

	default:
		return provider.NewError(provider.ErrProvider, body, err)
	}
}

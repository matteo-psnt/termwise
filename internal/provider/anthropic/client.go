package anthropic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	sdkoption "github.com/anthropics/anthropic-sdk-go/option"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// Client implements provider.AgentClient for the Anthropic API.
type Client struct {
	sdk sdk.Client
}

// New constructs an Anthropic client from a resolved provider.Config.
func New(cfg provider.Config) (*Client, error) {
	opts := []sdkoption.RequestOption{
		sdkoption.WithAPIKey(cfg.APIKey),
		sdkoption.WithRequestTimeout(provider.DefaultRequestTimeout),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, sdkoption.WithBaseURL(cfg.BaseURL))
	}
	return &Client{
		sdk: sdk.NewClient(opts...),
	}, nil
}

// ListModels fetches available models from the Anthropic API.
func (c *Client) ListModels(ctx context.Context) ([]provider.Model, error) {
	// The endpoint is paginated; the auto-pager follows has_more so models
	// past the first page are not silently missing from the picker.
	pager := c.sdk.Models.ListAutoPaging(ctx, sdk.ModelListParams{})
	var out []provider.Model
	for pager.Next() {
		m := pager.Current()
		out = append(out, provider.Model{
			ID:          m.ID,
			DisplayName: m.DisplayName,
		})
	}
	if err := pager.Err(); err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// Chat sends a multi-turn conversation with tool definitions and returns the response.
func (c *Client) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	msgs, err := toMessageParams(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("building messages: %w", err)
	}

	tools := toToolParams(req.Tools)
	params := toParams(req.Model, req.System, msgs, tools, req.Effort)

	msg, err := c.sdk.Messages.New(ctx, params)
	if isThinkingSignatureError(err) {
		// Replayed thinking blocks are bound to the conversation that
		// produced them, and newer accounts reject them once earlier history
		// was edited. The API accepts history without thinking, so retry
		// once without it rather than fail every later turn of the session.
		msgs, err = toMessageParams(withoutProviderState(req.Messages))
		if err != nil {
			return nil, fmt.Errorf("building messages: %w", err)
		}
		params.Messages = msgs
		msg, err = c.sdk.Messages.New(ctx, params)
	}
	if err != nil {
		return nil, mapErr(err)
	}
	return fromChatResponse(msg)
}

// thinkingSignatureMessage is what the API says when it rejects a replayed
// thinking block, e.g. "Invalid `signature` in `thinking` block".
const thinkingSignatureMessage = "`signature` in `thinking` block"

func isThinkingSignatureError(err error) bool {
	var apiErr *sdk.Error
	return errors.As(err, &apiErr) &&
		apiErr.StatusCode == http.StatusBadRequest &&
		strings.Contains(apiErr.Error(), thinkingSignatureMessage)
}

// withoutProviderState returns a copy of msgs with the thinking blocks
// carried in ProviderState removed; text, tool calls and results are kept.
func withoutProviderState(msgs []provider.Message) []provider.Message {
	out := make([]provider.Message, len(msgs))
	for i, m := range msgs {
		m.ProviderState = nil
		out[i] = m
	}
	return out
}

// mapErr converts SDK errors to our provider.Error type.
func mapErr(err error) error {
	// Anthropic SDK wraps HTTP errors as *sdk.Error (alias for *apierror.Error).
	return provider.MapError(displayName, err, func(err error) (int, bool) {
		var apiErr *sdk.Error
		if errors.As(err, &apiErr) {
			return apiErr.StatusCode, true
		}
		return 0, false
	})
}

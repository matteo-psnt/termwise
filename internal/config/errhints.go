package config

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/matteo-psnt/termwise/internal/cliname"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// keyPages is where each provider issues API keys, so an auth failure can say
// where to get a new one.
var keyPages = map[string]string{
	"anthropic":  "https://console.anthropic.com/settings/keys",
	"openai":     "https://platform.openai.com/api-keys",
	"groq":       "https://console.groq.com/keys",
	"deepseek":   "https://platform.deepseek.com/api_keys",
	"mistral":    "https://console.mistral.ai/api-keys",
	"openrouter": "https://openrouter.ai/settings/keys",
}

// startHints says how to start each self-hosted server.
var startHints = map[string]string{
	"ollama":   "Start it with `ollama serve`",
	"lmstudio": "Start its server from LM Studio's Developer tab or with `lms server start`",
}

// hintingClient rewrites provider errors into messages that say what failed
// and what to do next. The clients themselves only see a resolved key and
// URL, so they cannot say which env var or keychain entry a bad key came from.
type hintingClient struct {
	provider.AgentClient
	rc ResolvedConfig
}

func withErrorHints(c provider.AgentClient, rc ResolvedConfig) provider.AgentClient {
	return hintingClient{AgentClient: c, rc: rc}
}

func (c hintingClient) ListModels(ctx context.Context) ([]provider.Model, error) {
	ms, err := c.AgentClient.ListModels(ctx)
	return ms, explainError(c.rc, err)
}

func (c hintingClient) Complete(ctx context.Context, req provider.CompleteRequest) (*provider.CompleteResponse, error) {
	resp, err := c.AgentClient.Complete(ctx, req)
	return resp, explainError(c.rc, err)
}

func (c hintingClient) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	resp, err := c.AgentClient.Chat(ctx, req)
	return resp, explainError(c.rc, err)
}

// explainError returns err with its user-facing reason made actionable. The
// error kind and the underlying error are kept, so callers matching on
// either still work.
func explainError(rc ResolvedConfig, err error) error {
	pe, ok := errors.AsType[*provider.Error](err)
	if !ok {
		return err
	}
	name := displayName(rc.ProviderName)
	cmd := cliname.Name()

	switch {
	case pe.Kind == provider.ErrAuth:
		reason := name + " rejected the API key"
		if rc.KeySource != "" {
			reason += " from " + rc.KeySource
		}
		reason += " (invalid or expired)."
		if page := keyPages[rc.ProviderName]; page != "" {
			reason += "\n  Get a new key at " + page + ","
			reason += fmt.Sprintf(" then run `%s config` to use it.", cmd)
		} else {
			reason += fmt.Sprintf("\n  Run `%s config` to update it.", cmd)
		}
		return provider.NewError(pe.Kind, reason, pe.Err)

	case pe.Kind == provider.ErrModelNotFound:
		reason := fmt.Sprintf("%s has no model %q.\n  Run `%s config model list` to see what it offers, then `%s config model set <model>`.",
			name, rc.Model, cmd, cmd)
		return provider.NewError(pe.Kind, reason, pe.Err)

	case isConnectionError(pe.Err):
		return provider.NewError(pe.Kind, connectionReason(rc, name, cmd, pe.Err), pe.Err)
	}
	return err
}

func connectionReason(rc ResolvedConfig, name, cmd string, err error) string {
	if info, ok := provider.Lookup(rc.ProviderName); ok && info.NoAuth {
		where := info.ServerURL
		if rc.BaseURL != "" {
			where = rc.BaseURL
		}
		start := startHints[rc.ProviderName]
		if start == "" {
			start = "Start it"
		}
		return fmt.Sprintf("Can't reach %s at %s — is it running?\n  %s, or run `%s config` if it lives at another address.",
			name, where, start, cmd)
	}
	reason := "Can't reach " + name
	if rc.BaseURL != "" {
		reason += " at " + rc.BaseURL
	}
	return reason + fmt.Sprintf(": %v\n  Check your network connection. termwise uses HTTPS_PROXY if one is set.", unwrapNet(err))
}

// isConnectionError reports whether err is a failure to reach the server at
// all — refused, DNS, unreachable — rather than an error the server returned.
func isConnectionError(err error) bool {
	var opErr *net.OpError
	var dnsErr *net.DNSError
	return errors.As(err, &opErr) || errors.As(err, &dnsErr)
}

// unwrapNet strips the "Post <url>:" prefix the HTTP client adds, which
// repeats a URL the message already names.
func unwrapNet(err error) error {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return opErr
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr
	}
	return err
}

func displayName(name string) string {
	if info, ok := provider.Lookup(name); ok {
		return info.DisplayName
	}
	return name
}

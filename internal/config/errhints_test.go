package config

import (
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/matteo-psnt/termwise/internal/provider"
)

func TestExplainErrorAuthNamesKeySource(t *testing.T) {
	inner := errors.New("401")
	err := explainError(ResolvedConfig{ProviderName: "openai", KeySource: "$OPENAI_API_KEY"},
		provider.NewError(provider.ErrAuth, "API key invalid or expired.", inner))

	msg := err.Error()
	for _, want := range []string{"OpenAI", "$OPENAI_API_KEY", "https://platform.openai.com/api-keys"} {
		if !strings.Contains(msg, want) {
			t.Errorf("auth message %q does not mention %q", msg, want)
		}
	}
	pe, ok := errors.AsType[*provider.Error](err)
	if !ok || pe.Kind != provider.ErrAuth || !errors.Is(err, inner) {
		t.Errorf("explainError lost the error kind or the underlying error")
	}
}

func TestExplainErrorOllamaDown(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	err := explainError(ResolvedConfig{ProviderName: "ollama"},
		provider.NewError(provider.ErrProvider, "Post ...: connection refused", refused))
	if !strings.Contains(err.Error(), "ollama serve") {
		t.Errorf("message %q does not suggest starting Ollama", err)
	}
}

func TestExplainErrorLeavesOtherErrorsAlone(t *testing.T) {
	orig := provider.NewError(provider.ErrRateLimit, "Rate limited.", nil)
	if got := explainError(ResolvedConfig{ProviderName: "openai"}, orig); got.Error() != orig.Error() {
		t.Errorf("rate-limit error rewritten to %q", got)
	}
	if explainError(ResolvedConfig{}, nil) != nil {
		t.Error("nil error became non-nil")
	}
}

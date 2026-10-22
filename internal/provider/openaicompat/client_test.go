package openaicompat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// A refusal comes back as a 200 with empty content; it must not read as a
// blank answer.
func TestChatRefusalIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"c","object":"chat.completion","created":1,"model":"gpt-x","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":null,"refusal":"I can't help with that."}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer srv.Close()

	c, err := New("openai", srv.URL, provider.Config{APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Chat(context.Background(), provider.ChatRequest{Model: "gpt-x", Messages: []provider.Message{{Role: "user", Content: "hi"}}})
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Kind != provider.ErrRefused || pe.Reason != "OpenAI declined this request." {
		t.Fatalf("got (%+v, %v), want an OpenAI refusal error", resp, err)
	}
}

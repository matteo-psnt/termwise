package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/matteo-psnt/termwise/internal/provider"
)

func TestListModelsFollowsPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after_id") == "" {
			_, _ = fmt.Fprint(w, `{"data":[{"id":"m1","display_name":"M1","type":"model","created_at":"2025-01-01T00:00:00Z"}],"has_more":true,"first_id":"m1","last_id":"m1"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":[{"id":"m2","display_name":"M2","type":"model","created_at":"2025-01-01T00:00:00Z"}],"has_more":false,"first_id":"m2","last_id":"m2"}`)
	}))
	defer srv.Close()

	c, err := New(provider.Config{APIKey: "test", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "m1" || models[1].ID != "m2" {
		t.Fatalf("got %+v, want m1 and m2 from both pages", models)
	}
}

func TestRequestTimeoutCoversThinkingBudget(t *testing.T) {
	// A non-streaming request has to finish within the timeout, so it must
	// allow at least what the SDK expects for the largest max_tokens we send.
	p := toParams("claude-x", "", nil, nil, "high")
	want, err := sdk.CalculateNonStreamingTimeout(int(p.MaxTokens), p.Model, nil)
	if err != nil {
		t.Fatal(err)
	}
	if provider.DefaultRequestTimeout < want {
		t.Fatalf("DefaultRequestTimeout = %v, want at least %v for max_tokens %d", provider.DefaultRequestTimeout, want, p.MaxTokens)
	}
}

// okResponse is a minimal successful Messages API response.
const okResponse = `{"id":"msg_2","type":"message","role":"assistant","model":"claude-x","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1},"content":[{"type":"text","text":"done"}]}`

// thinkingHistory is a conversation whose assistant turn carries thinking.
func thinkingHistory() []provider.Message {
	return []provider.Message{
		{Role: "user", Content: "hi"},
		{
			Role:          "assistant",
			ToolCalls:     []provider.ToolCall{{ID: "tu_1", Name: "read", Input: map[string]any{"path": "x"}}},
			ProviderState: json.RawMessage(`[{"type":"thinking","thinking":"t","signature":"sig"},{"type":"redacted_thinking","data":"d"}]`),
		},
		{Role: "user", ToolResults: []provider.ToolResult{{ToolCallID: "tu_1", Content: "out"}}},
	}
}

func TestChatRetriesWithoutThinkingOnSignatureError(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"messages.1.content.0: Invalid `+"`signature` in `thinking`"+` block: bound to a different conversation"}}`)
			return
		}
		_, _ = fmt.Fprint(w, okResponse)
	}))
	defer srv.Close()

	c, err := New(provider.Config{APIKey: "test", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Chat(context.Background(), provider.ChatRequest{Model: "claude-x", Messages: thinkingHistory()})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "done" {
		t.Fatalf("got %+v, want the retried response", resp)
	}
	if !resp.HistoryStateDropped {
		t.Error("retried response does not tell the caller to drop the rejected state")
	}
	if len(bodies) != 2 {
		t.Fatalf("made %d requests, want 2", len(bodies))
	}
	if !strings.Contains(bodies[0], `"signature":"sig"`) {
		t.Fatalf("first request did not replay thinking: %s", bodies[0])
	}
	if strings.Contains(bodies[1], "thinking") {
		t.Fatalf("retry still carries thinking: %s", bodies[1])
	}
	for _, want := range []string{`"tool_use"`, `"tool_result"`, `"hi"`} {
		if !strings.Contains(bodies[1], want) {
			t.Fatalf("retry lost %s: %s", want, bodies[1])
		}
	}
}

func TestChatRetriesAtMostOnce(t *testing.T) {
	signature := "Invalid `signature` in `thinking` block"
	for _, tt := range []struct {
		message string
		want    int
	}{
		{signature, 2},             // one retry, and a second rejection is returned
		{"max_tokens: too big", 1}, // unrelated 400s are not retried
	} {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			body, _ := json.Marshal(map[string]any{"type": "error", "error": map[string]any{"type": "invalid_request_error", "message": tt.message}})
			_, _ = w.Write(body)
		}))

		c, err := New(provider.Config{APIKey: "test", BaseURL: srv.URL})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Chat(context.Background(), provider.ChatRequest{Model: "claude-x", Messages: thinkingHistory()}); err == nil {
			t.Fatalf("%q: want an error", tt.message)
		}
		srv.Close()
		if calls != tt.want {
			t.Fatalf("%q: made %d requests, want %d", tt.message, calls, tt.want)
		}
	}
}

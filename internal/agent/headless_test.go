package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// scriptedClient replays canned responses and records every request.
type scriptedClient struct {
	next     func(call int) *provider.ChatResponse
	requests []provider.ChatRequest
}

func (*scriptedClient) ListModels(context.Context) ([]provider.Model, error) { return nil, nil }

func (c *scriptedClient) Chat(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	c.requests = append(c.requests, req)
	return c.next(len(c.requests) - 1), nil
}

func readCall(id string) provider.ToolCall {
	return provider.ToolCall{ID: id, Name: "read", Input: map[string]any{"path": "/nonexistent/termwise-test"}}
}

func TestRunHeadlessCarriesProviderState(t *testing.T) {
	state := json.RawMessage(`[{"type":"thinking","thinking":"t","signature":"s"}]`)
	client := &scriptedClient{next: func(call int) *provider.ChatResponse {
		if call == 0 {
			return &provider.ChatResponse{ToolCalls: []provider.ToolCall{readCall("1")}, ProviderState: state}
		}
		return &provider.ChatResponse{Content: "done"}
	}}

	if _, err := RunHeadless(context.Background(), HeadlessConfig{Client: client, Prompt: "hi"}); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("got %d requests, want 2", len(client.requests))
	}
	asst := client.requests[1].Messages[1]
	if asst.Role != "assistant" || string(asst.ProviderState) != string(state) {
		t.Fatalf("assistant turn lost its provider state: %+v", asst)
	}
}

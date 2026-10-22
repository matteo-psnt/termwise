package agent

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
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

func TestRunHeadlessStopsALoopingModel(t *testing.T) {
	client := &scriptedClient{next: func(call int) *provider.ChatResponse {
		return &provider.ChatResponse{ToolCalls: []provider.ToolCall{readCall(strconv.Itoa(call))}}
	}}
	_, err := RunHeadless(context.Background(), HeadlessConfig{Client: client, Prompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "no final response") {
		t.Fatalf("got err %v, want a step-limit error", err)
	}
	if len(client.requests) != maxHeadlessSteps {
		t.Fatalf("got %d requests, want %d", len(client.requests), maxHeadlessSteps)
	}
}

func TestRunHeadlessRejectsEmptyCommand(t *testing.T) {
	client := &scriptedClient{next: func(int) *provider.ChatResponse {
		return &provider.ChatResponse{ToolCalls: []provider.ToolCall{
			{ID: "1", Name: "command", Input: map[string]any{"content": "  "}},
		}}
	}}
	if out, err := RunHeadless(context.Background(), HeadlessConfig{Client: client, Prompt: "hi"}); err == nil {
		t.Fatalf("got %+v, want an error for an empty command", out)
	}
}

// Once the provider reports that it dropped the replayed state, the loop must
// stop sending it, or every later request is rejected and retried again.
func TestRunHeadlessDropsStateTheProviderRejected(t *testing.T) {
	state := json.RawMessage(`[{"type":"thinking","thinking":"t","signature":"s"}]`)
	client := &scriptedClient{next: func(call int) *provider.ChatResponse {
		switch call {
		case 0:
			return &provider.ChatResponse{ToolCalls: []provider.ToolCall{readCall("1")}, ProviderState: state}
		case 1:
			return &provider.ChatResponse{ToolCalls: []provider.ToolCall{readCall("2")}, HistoryStateDropped: true}
		}
		return &provider.ChatResponse{Content: "done"}
	}}

	if _, err := RunHeadless(context.Background(), HeadlessConfig{Client: client, Prompt: "hi"}); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 3 {
		t.Fatalf("got %d requests, want 3", len(client.requests))
	}
	for i, m := range client.requests[2].Messages {
		if len(m.ProviderState) != 0 {
			t.Errorf("message %d still replays state the provider rejected", i)
		}
	}
}

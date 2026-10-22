package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// A tool-use turn with thinking enabled, as the API returns it.
const thinkingToolUseResponse = `{
	"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-x",
	"stop_reason": "tool_use",
	"usage": {"input_tokens": 10, "output_tokens": 20},
	"content": [
		{"type": "thinking", "thinking": "let me look", "signature": "sig-abc"},
		{"type": "redacted_thinking", "data": "opaque-data"},
		{"type": "text", "text": "Checking."},
		{"type": "tool_use", "id": "tu_1", "name": "read", "input": {"path": "go.mod"}}
	]
}`

func TestThinkingBlocksRoundTrip(t *testing.T) {
	var msg sdk.Message
	if err := json.Unmarshal([]byte(thinkingToolUseResponse), &msg); err != nil {
		t.Fatal(err)
	}
	resp, err := fromChatResponse(&msg)
	if err != nil {
		t.Fatal(err)
	}

	// The assistant message goes through session persistence before it is
	// sent back, so round-trip it through JSON as agentui's session save does.
	saved, err := json.Marshal(provider.Message{
		Role:          "assistant",
		Content:       resp.Content,
		ToolCalls:     resp.ToolCalls,
		ProviderState: resp.ProviderState,
	})
	if err != nil {
		t.Fatal(err)
	}
	var loaded provider.Message
	if err := json.Unmarshal(saved, &loaded); err != nil {
		t.Fatal(err)
	}

	p, err := toAssistantMessageParam(loaded)
	if err != nil {
		t.Fatal(err)
	}
	// Check the wire form, which is what the API validates.
	wire, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Content []struct {
			Type string `json:"type"`
		} `json:"content"`
	}
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, b := range decoded.Content {
		types = append(types, b.Type)
	}
	if got, want := strings.Join(types, ","), "thinking,redacted_thinking,text,tool_use"; got != want {
		t.Fatalf("block order = %s, want %s", got, want)
	}
	if th := p.Content[0].OfThinking; th == nil || th.Signature != "sig-abc" || th.Thinking != "let me look" {
		t.Fatalf("thinking block not passed back unchanged: %+v", p.Content[0])
	}
	if rd := p.Content[1].OfRedactedThinking; rd == nil || rd.Data != "opaque-data" {
		t.Fatalf("redacted thinking block not passed back unchanged: %+v", p.Content[1])
	}
}

func TestSessionWithoutProviderStateLoads(t *testing.T) {
	// Sessions saved before ProviderState existed have no such key.
	var m provider.Message
	if err := json.Unmarshal([]byte(`{"Role":"assistant","Content":"hi","ToolCalls":null,"ToolResults":null}`), &m); err != nil {
		t.Fatal(err)
	}
	p, err := toAssistantMessageParam(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Content) != 1 || p.Content[0].OfText == nil {
		t.Fatalf("got %+v, want a single text block", p.Content)
	}
}

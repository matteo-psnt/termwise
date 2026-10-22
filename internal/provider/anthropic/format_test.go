package anthropic

import (
	"encoding/json"
	"errors"
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
	// The API verifies the signature against the block, so the wire form of
	// each block must be exactly what was received.
	for i, want := range []string{
		`{"signature":"sig-abc","thinking":"let me look","type":"thinking"}`,
		`{"data":"opaque-data","type":"redacted_thinking"}`,
	} {
		got, err := json.Marshal(p.Content[i])
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("block %d on the wire = %s, want %s", i, got, want)
		}
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

func TestMaxTokens(t *testing.T) {
	// Current models think even without effort, so the default must leave
	// room for thinking as well as the answer.
	if got := toParams("m", "", nil, nil, "").MaxTokens; got != 16000 {
		t.Fatalf("default max_tokens = %d, want 16000", got)
	}
	if got := toParams("m", "", nil, nil, "high").MaxTokens; got != 20000 {
		t.Fatalf("thinking max_tokens = %d, want 20000", got)
	}
}

func TestToolResultsPrecedeUserText(t *testing.T) {
	p, err := toUserMessageParam(provider.Message{
		Role:        "user",
		Content:     "and also",
		ToolResults: []provider.ToolResult{{ToolCallID: "tu_1", Content: "out"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Content) != 2 || p.Content[0].OfToolResult == nil || p.Content[1].OfText == nil {
		t.Fatalf("got %+v, want tool_result then text", p.Content)
	}
}

func TestNoEmptyTextBlocks(t *testing.T) {
	asst, err := toAssistantMessageParam(provider.Message{
		Role:      "assistant",
		Content:   "\n\n",
		ToolCalls: []provider.ToolCall{{ID: "tu_1", Name: "read", Input: map[string]any{}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(asst.Content) != 1 || asst.Content[0].OfToolUse == nil {
		t.Fatalf("whitespace-only assistant text was sent: %+v", asst.Content)
	}

	user, err := toUserMessageParam(provider.Message{
		Role:        "user",
		ToolResults: []provider.ToolResult{{ToolCallID: "tu_1", Content: ""}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tr := user.Content[0].OfToolResult; tr == nil || len(tr.Content) != 0 {
		t.Fatalf("empty tool result should carry no text block: %+v", user.Content[0])
	}
}

func TestEmptyAssistantTurnIsDropped(t *testing.T) {
	msgs, err := toMessageParams([]provider.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant"},
		{Role: "user", Content: "again"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Role != sdk.MessageParamRoleUser || msgs[1].Role != sdk.MessageParamRoleUser {
		t.Fatalf("got %+v, want the two user turns only", msgs)
	}
}

func TestRefusalIsAnError(t *testing.T) {
	for _, tt := range []struct{ body, reason string }{
		{`{"id":"m","type":"message","role":"assistant","model":"x","stop_reason":"refusal","stop_details":{"type":"refusal","category":"cyber","explanation":"no"},"usage":{},"content":[]}`,
			"Anthropic declined this request (category: cyber)."},
		{`{"id":"m","type":"message","role":"assistant","model":"x","stop_reason":"refusal","stop_details":{"type":"refusal","category":null,"explanation":null},"usage":{},"content":[]}`,
			"Anthropic declined this request."},
	} {
		var msg sdk.Message
		if err := json.Unmarshal([]byte(tt.body), &msg); err != nil {
			t.Fatal(err)
		}
		resp, err := fromChatResponse(&msg)
		var pe *provider.Error
		if !errors.As(err, &pe) || pe.Kind != provider.ErrRefused || pe.Reason != tt.reason {
			t.Fatalf("got (%+v, %v), want refusal %q", resp, err, tt.reason)
		}
	}
}

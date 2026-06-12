package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// toParams builds the MessageNewParams for both Complete and Chat calls.
// system and messages are already in their final form; tools may be nil.
func toParams(model, system string, messages []sdk.MessageParam, tools []sdk.ToolUnionParam) sdk.MessageNewParams {
	p := sdk.MessageNewParams{
		Model:     sdk.Model(model),
		MaxTokens: 8096,
		Messages:  messages,
	}
	if system != "" {
		p.System = []sdk.TextBlockParam{{Text: system}}
	}
	if len(tools) > 0 {
		p.Tools = tools
	}
	return p
}

// toSingleMessage converts a single-shot prompt into a one-element message slice.
func toSingleMessage(prompt string) []sdk.MessageParam {
	return []sdk.MessageParam{
		sdk.NewUserMessage(sdk.ContentBlockParamUnion{
			OfText: &sdk.TextBlockParam{Text: prompt},
		}),
	}
}

// toMessageParams converts our internal Message slice to SDK MessageParam slice.
func toMessageParams(msgs []provider.Message) ([]sdk.MessageParam, error) {
	out := make([]sdk.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		p, err := toMessageParam(m)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func toMessageParam(m provider.Message) (sdk.MessageParam, error) {
	switch m.Role {
	case "user":
		return toUserMessageParam(m)
	case "assistant":
		return toAssistantMessageParam(m)
	default:
		return sdk.MessageParam{}, fmt.Errorf("unknown message role: %q", m.Role)
	}
}

func toUserMessageParam(m provider.Message) (sdk.MessageParam, error) {
	var blocks []sdk.ContentBlockParamUnion

	if m.Content != "" {
		blocks = append(blocks, sdk.ContentBlockParamUnion{
			OfText: &sdk.TextBlockParam{Text: m.Content},
		})
	}

	for _, tr := range m.ToolResults {
		content := []sdk.ToolResultBlockParamContentUnion{
			{OfText: &sdk.TextBlockParam{Text: tr.Content}},
		}
		block := sdk.ToolResultBlockParam{
			ToolUseID: tr.ToolCallID,
			Content:   content,
		}
		if tr.IsError {
			block.IsError = param.NewOpt(true)
		}
		blocks = append(blocks, sdk.ContentBlockParamUnion{
			OfToolResult: &block,
		})
	}

	return sdk.MessageParam{
		Role:    sdk.MessageParamRoleUser,
		Content: blocks,
	}, nil
}

func toAssistantMessageParam(m provider.Message) (sdk.MessageParam, error) {
	var blocks []sdk.ContentBlockParamUnion

	if m.Content != "" {
		blocks = append(blocks, sdk.ContentBlockParamUnion{
			OfText: &sdk.TextBlockParam{Text: m.Content},
		})
	}

	for _, tc := range m.ToolCalls {
		blocks = append(blocks, sdk.ContentBlockParamUnion{
			OfToolUse: &sdk.ToolUseBlockParam{
				ID:    tc.ID,
				Name:  tc.Name,
				Input: tc.Input,
			},
		})
	}

	return sdk.MessageParam{
		Role:    sdk.MessageParamRoleAssistant,
		Content: blocks,
	}, nil
}

// toToolParams converts our internal ToolDef slice to SDK ToolUnionParam slice.
func toToolParams(tools []provider.ToolDef) []sdk.ToolUnionParam {
	out := make([]sdk.ToolUnionParam, len(tools))
	for i, t := range tools {
		tp := sdk.ToolParam{
			Name:        t.Name,
			InputSchema: sdk.ToolInputSchemaParam{},
		}
		if t.Description != "" {
			tp.Description = param.NewOpt(t.Description)
		}
		// Copy JSON Schema fields into the InputSchema.
		// The SDK's ToolInputSchemaParam uses ExtraFields for additional properties.
		if props, ok := t.InputSchema["properties"]; ok {
			tp.InputSchema.Properties = props
		}
		if req, ok := t.InputSchema["required"]; ok {
			if reqSlice, ok := req.([]string); ok {
				tp.InputSchema.Required = reqSlice
			}
		}
		out[i] = sdk.ToolUnionParam{OfTool: &tp}
	}
	return out
}

// fromResponse extracts the text content and usage from a Message response.
// Used for Complete (single-shot) calls.
func fromResponse(msg *sdk.Message) *provider.CompleteResponse {
	text := extractText(msg.Content)
	return &provider.CompleteResponse{
		Content:      text,
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
	}
}

// fromChatResponse extracts content, tool calls, stop reason, and usage.
// Used for Chat (agent) calls.
func fromChatResponse(msg *sdk.Message) (*provider.ChatResponse, error) {
	resp := &provider.ChatResponse{
		StopReason:   string(msg.StopReason),
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
	}

	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			resp.Content += block.Text
		case "tool_use":
			input, err := parseToolInput(block.Input)
			if err != nil {
				return nil, fmt.Errorf("failed to parse tool input for %q: %w", block.Name, err)
			}
			resp.ToolCalls = append(resp.ToolCalls, provider.ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: input,
			})
		}
		// Other block types (thinking, etc.) are silently ignored.
	}

	return resp, nil
}

// extractText concatenates all text blocks in a content slice.
func extractText(blocks []sdk.ContentBlockUnion) string {
	var b strings.Builder
	for _, block := range blocks {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	return b.String()
}

// parseToolInput unmarshals the raw JSON input into a map.
func parseToolInput(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// toParams builds MessageNewParams and enables adaptive thinking when effort is set.
// max_tokens caps the combined thinking + visible output budget. Current
// models think even when no thinking config is sent, so even the default has
// to leave room for thinking ahead of the answer; 16000 is the documented
// figure for non-streaming requests. With effort set the model may think
// longer, so the cap is raised, but only as far as a non-streaming request
// can finish within DefaultRequestTimeout by the SDK's own estimate: 32000
// would need streaming.
func toParams(model, system string, messages []sdk.MessageParam, tools []sdk.ToolUnionParam, effort string) sdk.MessageNewParams {
	const (
		maxTokensDefault  = 16000
		maxTokensThinking = 20000
	)
	maxTokens := int64(maxTokensDefault)
	if effort != "" {
		maxTokens = maxTokensThinking
	}
	p := sdk.MessageNewParams{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  messages,
	}
	if system != "" {
		p.System = []sdk.TextBlockParam{{Text: system}}
	}
	if len(tools) > 0 {
		p.Tools = tools
	}
	if effort != "" {
		p.Thinking = sdk.ThinkingConfigParamUnion{OfAdaptive: &sdk.ThinkingConfigAdaptiveParam{}}
		p.OutputConfig = sdk.OutputConfigParam{Effort: sdk.OutputConfigEffort(effort)}
	}
	return p
}

// toMessageParams converts our internal Message slice to SDK MessageParam slice.
func toMessageParams(msgs []provider.Message) ([]sdk.MessageParam, error) {
	out := make([]sdk.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		p, err := toMessageParam(m)
		if err != nil {
			return nil, err
		}
		// A model can end a turn with no content at all, and the API rejects
		// an empty message on every later request, which would wedge the
		// conversation. Dropping it is safe: the API merges the consecutive
		// user turns this leaves.
		if len(p.Content) == 0 {
			continue
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

	// tool_result blocks first: the API rejects a user turn that answers
	// tool_use with any text ahead of the results.
	for _, tr := range m.ToolResults {
		block := sdk.ToolResultBlockParam{ToolUseID: tr.ToolCallID}
		// Content is optional, but an empty text block is rejected, and a
		// tool can legitimately return nothing (e.g. reading an empty file).
		if hasText(tr.Content) {
			block.Content = []sdk.ToolResultBlockParamContentUnion{
				{OfText: &sdk.TextBlockParam{Text: tr.Content}},
			}
		}
		if tr.IsError {
			block.IsError = param.NewOpt(true)
		}
		blocks = append(blocks, sdk.ContentBlockParamUnion{
			OfToolResult: &block,
		})
	}

	if hasText(m.Content) {
		blocks = append(blocks, sdk.ContentBlockParamUnion{
			OfText: &sdk.TextBlockParam{Text: m.Content},
		})
	}

	return sdk.MessageParam{
		Role:    sdk.MessageParamRoleUser,
		Content: blocks,
	}, nil
}

func toAssistantMessageParam(m provider.Message) (sdk.MessageParam, error) {
	if replay, ok, err := decodeReplayState(m.ProviderState); err != nil {
		return sdk.MessageParam{}, err
	} else if ok {
		return sdk.MessageParam{Role: sdk.MessageParamRoleAssistant, Content: replay}, nil
	}

	var blocks []sdk.ContentBlockParamUnion

	// Sessions saved before the whole turn was kept carry only its thinking
	// blocks. The API needs them, signatures intact, ahead of the tool_use
	// blocks when the loop continues with tool results.
	if len(m.ProviderState) > 0 {
		var thinking []sdk.ContentBlockParamUnion
		if err := json.Unmarshal(m.ProviderState, &thinking); err != nil {
			return sdk.MessageParam{}, fmt.Errorf("decoding thinking blocks: %w", err)
		}
		blocks = append(blocks, thinking...)
	}

	if hasText(m.Content) {
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

// hasText reports whether s is worth a text block. The API rejects text
// blocks that are empty or only whitespace, and a model can return just "\n"
// ahead of a tool call, which would otherwise be echoed back and fail.
func hasText(s string) bool {
	return strings.TrimSpace(s) != ""
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

// fromChatResponse extracts content, tool calls, stop reason, and usage.
func fromChatResponse(msg *sdk.Message) (*provider.ChatResponse, error) {
	// A refusal arrives as a successful response, possibly with no content.
	if msg.StopReason == sdk.StopReasonRefusal {
		return nil, provider.Refusal(displayName, string(msg.StopDetails.Category), msg.StopDetails.Explanation)
	}
	resp := &provider.ChatResponse{
		StopReason:   string(msg.StopReason),
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
	}

	var all []sdk.ContentBlockParamUnion
	hasThinking := false
	for _, block := range msg.Content {
		all = append(all, block.ToParam())
		switch block.Type {
		case "thinking", "redacted_thinking":
			hasThinking = true
		case "text":
			resp.Content += block.Text
		case "tool_use":
			input, err := provider.ParseToolInput(block.Input)
			if err != nil {
				return nil, fmt.Errorf("failed to parse tool input for %q: %w", block.Name, err)
			}
			resp.ToolCalls = append(resp.ToolCalls, provider.ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: input,
			})
		}
		// Other block types are silently ignored.
	}

	if hasThinking {
		state, err := json.Marshal(replayState{Blocks: all})
		if err != nil {
			return nil, fmt.Errorf("encoding the turn for replay: %w", err)
		}
		resp.ProviderState = state
	}
	return resp, nil
}

// replayState is an assistant turn kept exactly as the API returned it, so
// it can be sent back unchanged. A thinking block's signature covers the
// conversation it was produced in, and the API treats a turn rebuilt in a
// different order (thinking hoisted ahead of interleaved text and tool_use
// blocks, text blocks merged) as an edit that invalidates its thinking. It is
// only kept for turns with thinking; others rebuild identically from
// Content and ToolCalls.
type replayState struct {
	Blocks []sdk.ContentBlockParamUnion `json:"blocks"`
}

// decodeReplayState returns the blocks of a turn stored whole. ok is false
// for no state, or for the older form (a JSON array of only the thinking
// blocks), which toAssistantMessageParam rebuilds around.
func decodeReplayState(state json.RawMessage) ([]sdk.ContentBlockParamUnion, bool, error) {
	trimmed := bytes.TrimSpace(state)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, false, nil
	}
	var rs replayState
	if err := json.Unmarshal(trimmed, &rs); err != nil {
		return nil, false, fmt.Errorf("decoding the stored turn: %w", err)
	}
	blocks := make([]sdk.ContentBlockParamUnion, 0, len(rs.Blocks))
	for _, b := range rs.Blocks {
		// The API returns whitespace-only text (a "\n" before a tool call)
		// but rejects it in requests, so that one block can't go back as is.
		if b.OfText != nil && !hasText(b.OfText.Text) {
			continue
		}
		blocks = append(blocks, b)
	}
	return blocks, true, nil
}

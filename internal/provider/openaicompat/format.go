package openaicompat

import (
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// toParams builds ChatCompletionNewParams for both Complete and Chat calls.
// System prompt is prepended as a system message if non-empty.
func toParams(model, system string, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) openai.ChatCompletionNewParams {
	p := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(model),
		Messages: messages,
	}
	if system != "" {
		p.Messages = append([]openai.ChatCompletionMessageParamUnion{openai.SystemMessage(system)}, messages...)
	}
	if len(tools) > 0 {
		p.Tools = tools
	}
	return p
}

// toMessageParams converts our internal Message slice to SDK message params.
// A single provider.Message with tool results expands into multiple tool messages.
func toMessageParams(msgs []provider.Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	var out []openai.ChatCompletionMessageParamUnion
	for _, m := range msgs {
		params, err := toMessageParam(m)
		if err != nil {
			return nil, err
		}
		out = append(out, params...)
	}
	return out, nil
}

func toMessageParam(m provider.Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	switch m.Role {
	case "user":
		return toUserMessageParams(m), nil
	case "assistant":
		p, err := toAssistantMessageParam(m)
		if err != nil {
			return nil, err
		}
		return []openai.ChatCompletionMessageParamUnion{p}, nil
	default:
		return nil, fmt.Errorf("unknown message role: %q", m.Role)
	}
}

// toUserMessageParams converts a user message into one or more SDK messages.
// Tool results each become a separate tool message (OpenAI format requires this).
func toUserMessageParams(m provider.Message) []openai.ChatCompletionMessageParamUnion {
	var out []openai.ChatCompletionMessageParamUnion
	if m.Content != "" {
		out = append(out, openai.UserMessage(m.Content))
	}
	for _, tr := range m.ToolResults {
		out = append(out, openai.ToolMessage(tr.Content, tr.ToolCallID))
	}
	return out
}

func toAssistantMessageParam(m provider.Message) (openai.ChatCompletionMessageParamUnion, error) {
	var asst openai.ChatCompletionAssistantMessageParam
	if m.Content != "" {
		asst.Content.OfString = openai.String(m.Content)
	}
	for _, tc := range m.ToolCalls {
		argsJSON, err := json.Marshal(tc.Input)
		if err != nil {
			return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("marshal tool call %q input: %w", tc.Name, err)
		}
		asst.ToolCalls = append(asst.ToolCalls, openai.ChatCompletionMessageToolCallParam{
			ID: tc.ID,
			Function: openai.ChatCompletionMessageToolCallFunctionParam{
				Name:      tc.Name,
				Arguments: string(argsJSON),
			},
		})
	}
	return openai.ChatCompletionMessageParamUnion{OfAssistant: &asst}, nil
}

// toToolParams converts our internal ToolDef slice to SDK tool params.
func toToolParams(tools []provider.ToolDef) []openai.ChatCompletionToolParam {
	out := make([]openai.ChatCompletionToolParam, len(tools))
	for i, t := range tools {
		fn := shared.FunctionDefinitionParam{
			Name:       t.Name,
			Parameters: shared.FunctionParameters(t.InputSchema),
		}
		if t.Description != "" {
			fn.Description = openai.String(t.Description)
		}
		out[i] = openai.ChatCompletionToolParam{Function: fn}
	}
	return out
}

// fromResponse extracts content and usage from a non-streaming chat completion.
// Used for Complete (single-shot) calls.
func fromResponse(msg *openai.ChatCompletion) *provider.CompleteResponse {
	var content string
	if len(msg.Choices) > 0 {
		content = msg.Choices[0].Message.Content
	}
	return &provider.CompleteResponse{
		Content:      content,
		InputTokens:  int(msg.Usage.PromptTokens),
		OutputTokens: int(msg.Usage.CompletionTokens),
	}
}

// fromChatResponse extracts content, tool calls, stop reason, and usage.
// Used for Chat (agent) calls.
func fromChatResponse(msg *openai.ChatCompletion) (*provider.ChatResponse, error) {
	resp := &provider.ChatResponse{
		InputTokens:  int(msg.Usage.PromptTokens),
		OutputTokens: int(msg.Usage.CompletionTokens),
	}
	if len(msg.Choices) == 0 {
		return resp, nil
	}
	choice := msg.Choices[0]
	resp.StopReason = choice.FinishReason
	resp.Content = choice.Message.Content
	for _, tc := range choice.Message.ToolCalls {
		input, err := parseToolInput(tc.Function.Arguments)
		if err != nil {
			return nil, fmt.Errorf("parse tool input for %q: %w", tc.Function.Name, err)
		}
		resp.ToolCalls = append(resp.ToolCalls, provider.ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}
	return resp, nil
}

func parseToolInput(args string) (map[string]any, error) {
	if args == "" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// filterModels applies per-provider filtering to remove noisy/irrelevant models.
// Only openai gets filtered — other providers return curated lists already.
func filterModels(providerName string, models []provider.Model) []provider.Model {
	if providerName != "openai" {
		return models
	}
	// Keep only chat-capable model families; drop embeddings, TTS, image, legacy, etc.
	keepPrefixes := []string{"gpt-", "o1", "o3", "o4", "chatgpt-"}
	var out []provider.Model
	for _, m := range models {
		for _, prefix := range keepPrefixes {
			if strings.HasPrefix(m.ID, prefix) {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

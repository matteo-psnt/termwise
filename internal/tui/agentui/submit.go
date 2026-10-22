package agentui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// handleInitialPrompt submits the prompt given on the command line once the
// program is running. It does nothing unless the TUI is still idle.
func (m Model) handleInitialPrompt(prompt string) (tea.Model, tea.Cmd) {
	if m.state != stateIdle {
		return m, nil
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return m, nil
	}
	m.initialPrompt = ""
	m.rememberPrompt(prompt)
	return m.submitMessage(prompt)
}

// submitCurrentInput handles Enter in idle: it sends the input as a message,
// or runs it as a slash command when it starts with "/".
func (m Model) submitCurrentInput() (tea.Model, tea.Cmd) {
	// Backslash-Enter is the portable newline fallback: a trailing "\" turns
	// Enter into a line break instead of a submit, for terminals that can't
	// report a real Shift+Enter.
	if v := m.input.Value(); strings.HasSuffix(v, "\\") {
		m.input.SetValue(v[:len(v)-1] + "\n")
		m.input.MoveToEnd()
		return m, nil
	}
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return m, nil
	}
	m.input.SetValue("")
	m.cancelHistoryNav()
	m.rememberPrompt(text)
	if strings.HasPrefix(text, "/") {
		return m.dispatchSlashCommand(text)
	}
	return m.submitMessage(text)
}

// submitMessage adds the user's message and starts a model request.
func (m Model) submitMessage(text string) (tea.Model, tea.Cmd) {
	m.resetSuggestionContext()
	m.clearSuggestion()
	m.clearShellCommand()

	// Attach stdin to first user message if present.
	content := text
	if m.stdin != "" {
		content = text + "\n\n" + m.stdin
		m.stdin = ""
	}

	m.appendThreadEntries(UserEntry{Content: text})
	m.messages = closeDanglingToolCalls(m.messages)
	m.messages = append(m.messages, provider.Message{Role: "user", Content: content})
	m.messages = trimContext(m.messages, m.contextWindow)
	m.beginGeneration()
	m.state = stateThinking
	m.refreshViewport()

	return m, m.startChat()
}

// notRunResult is the tool result recorded for a tool call that never ran.
const notRunResult = "Not run: the turn ended before this tool call was processed."

// closeDanglingToolCalls gives every tool call in the history a result.
//
// Providers reject a history where an assistant tool call is not answered in
// the very next message, and a turn can end with calls unanswered: Esc while a
// tool runs (or the model is mid-loop), quitting at an approval prompt and
// resuming, or the command tool ending the turn before the calls after it.
// Without this every later message in the session fails. Missing results are
// added to the tool-result message that follows, or as a new one. The input is
// not modified.
func closeDanglingToolCalls(messages []provider.Message) []provider.Message {
	out := make([]provider.Message, 0, len(messages)+1)
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		out = append(out, msg)
		if msg.Role != "assistant" || len(msg.ToolCalls) == 0 {
			continue
		}

		var next *provider.Message
		if i+1 < len(messages) && messages[i+1].Role == "user" && len(messages[i+1].ToolResults) > 0 {
			next = &messages[i+1]
		}
		answered := make(map[string]bool)
		if next != nil {
			for _, tr := range next.ToolResults {
				answered[tr.ToolCallID] = true
			}
		}
		var missing []provider.ToolResult
		for _, tc := range msg.ToolCalls {
			if !answered[tc.ID] {
				missing = append(missing, provider.ToolResult{ToolCallID: tc.ID, Content: notRunResult, IsError: true})
			}
		}

		switch {
		case len(missing) == 0:
		case next != nil:
			patched := *next
			patched.ToolResults = slices.Concat(next.ToolResults, missing)
			out = append(out, patched)
			i++
		default:
			out = append(out, provider.Message{Role: "user", ToolResults: missing})
		}
	}
	return out
}

// chatRequest builds the ChatRequest from current state.
func (m Model) chatRequest() provider.ChatRequest {
	return provider.ChatRequest{
		Model:    m.modelID,
		System:   m.system,
		Messages: m.messages,
		Tools:    m.toolDefs,
		Effort:   m.effectiveEffort(),
	}
}

// effectiveEffort returns the effort to send: configured value, or the default
// for thinking-capable models, or empty for models without thinking support.
func (m Model) effectiveEffort() string {
	return config.EffectiveEffort(m.providerName, m.modelID, m.effort)
}

// trimContext drops oldest tool results when approaching the context window limit.
//
// The returned slice is a new outer slice, but the copy is shallow: each
// message's ToolResults still shares its backing array with the input, so a
// dropped result is overwritten in the caller's messages too.
func trimContext(messages []provider.Message, contextWindow int) []provider.Message {
	const (
		fixedTokens   = 700 // system prompt + tool defs estimate
		charsPerToken = 4
		threshold     = 0.70
	)

	limit := int(float64(contextWindow) * threshold)

	estimate := func(msgs []provider.Message) int {
		total := fixedTokens
		for _, m := range msgs {
			total += len(m.Content) / charsPerToken
			for _, tr := range m.ToolResults {
				total += len(tr.Content) / charsPerToken
			}
			for _, tc := range m.ToolCalls {
				for _, v := range tc.Input {
					total += len(fmt.Sprintf("%v", v)) / charsPerToken
				}
			}
		}
		return total
	}

	if estimate(messages) <= limit {
		return messages
	}

	// Drop oldest tool results first.
	const dropped = "[result dropped — conversation too long. You can re-read this file if needed.]"
	out := make([]provider.Message, len(messages))
	copy(out, messages)

	for i := range out {
		if len(out[i].ToolResults) > 0 {
			// copy(out, messages) shares each ToolResults backing array with
			// the caller; clone before rewriting so the session's own history
			// (what gets saved and resumed) keeps the full results.
			out[i].ToolResults = slices.Clone(out[i].ToolResults)
		}
		for j := range out[i].ToolResults {
			if out[i].ToolResults[j].Content != dropped {
				out[i].ToolResults[j].Content = dropped
				if estimate(out) <= limit {
					return out
				}
			}
		}
	}

	return out
}

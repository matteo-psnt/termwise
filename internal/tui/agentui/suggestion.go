package agentui

import (
	"context"
	"errors"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/provider"
)

var (
	suggestionWrappedMetaPattern    = regexp.MustCompile(`^\(.*\)$|^\[.*\]$`)
	suggestionMultiSentencePattern  = regexp.MustCompile(`[.!?]\s+[A-Z]`)
	suggestionEvaluativePattern     = regexp.MustCompile(`^done$|thanks|thank you|looks good|sounds good|that works|that worked|that's all|nice|great|perfect|makes sense|awesome|excellent`)
	suggestionAssistantVoicePattern = regexp.MustCompile(`(?i)^(let me|i'll|i've|i'm|i can|i would|i think|i notice|here's|here is|here are|that's|this is|this will|you can|you should|you could|sure,|of course|certainly)`)
)

const suggestionSystemPrompt = `[SUGGESTION MODE: Suggest what the user might naturally type next into termwise.]

FIRST: Look at the user's recent messages and original request.

Your job is to predict what THEY would type - not what you think they should do.

THE TEST: Would they think "I was just about to type that"?

EXAMPLES:
User asked "fix the bug and run tests", bug is fixed -> "run the tests"
After code written -> "try it out"
Assistant offers options -> suggest the one the user would likely pick, based on conversation
Assistant asks to continue -> "yes" or "go ahead"
Task complete, obvious follow-up -> "commit this" or "push it"
After error or misunderstanding -> silence (let them assess/correct)

Be specific: "run the tests" beats "continue".

NEVER SUGGEST:
- Evaluative ("looks good", "thanks")
- Questions ("what about...?")
- Assistant-voice ("Let me...", "I'll...", "Here's...")
- New ideas they didn't ask about
- Multiple sentences

Stay silent if the next step isn't obvious from what the user said.

Format: 2-15 words, match the user's style. Or nothing.

Reply with ONLY the suggestion, no quotes or explanation.`

type suggestionMsg struct {
	generation uint64
	text       string
	err        error
}

func (m *Model) resetSuggestionContext() {
	m.suggestionCancel()
	m.suggestionCtx, m.suggestionCancel = newSuggestionContext()
}

func (m *Model) clearSuggestion() {
	m.suggestion = ""
	m.activeSuggestion = 0
}

func (m Model) visibleSuggestion() string {
	if m.state != stateIdle {
		return ""
	}
	if strings.TrimSpace(m.input.Value()) != "" {
		return ""
	}
	return m.suggestion
}

func (m *Model) startSuggestion() tea.Cmd {
	if reason := m.suggestionSuppressReason(); reason != "" {
		return nil
	}

	prompt := buildSuggestionPrompt(m.thread)
	if prompt == "" {
		return nil
	}

	m.resetSuggestionContext()
	m.suggestion = ""
	m.nextSuggestion++
	m.activeSuggestion = m.nextSuggestion

	req := provider.CompleteRequest{
		Model:  m.modelID,
		System: suggestionSystemPrompt,
		Prompt: prompt,
	}
	generation := m.activeSuggestion
	ctx := m.suggestionCtx
	client := m.provider

	return func() tea.Msg {
		resp, err := client.Complete(ctx, req)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return suggestionMsg{generation: generation}
			}
			return suggestionMsg{generation: generation, err: err}
		}
		text := ""
		if resp != nil {
			text = strings.TrimSpace(resp.Content)
		}
		return suggestionMsg{generation: generation, text: text}
	}
}

func (m Model) suggestionSuppressReason() string {
	if !m.suggestions {
		return "disabled"
	}
	if m.state != stateIdle {
		return "not_idle"
	}
	if len(m.messages) == 0 {
		return "no_messages"
	}
	if countRoleMessages(m.messages, "assistant") == 0 {
		return "no_assistant_turn"
	}
	if !hasSuggestionWorthyEntry(m.thread) {
		return "no_visible_reply"
	}
	return ""
}

func countRoleMessages(messages []provider.Message, role string) int {
	total := 0
	for _, msg := range messages {
		if msg.Role == role {
			total++
		}
	}
	return total
}

func hasSuggestionWorthyEntry(entries []ThreadEntry) bool {
	for i := len(entries) - 1; i >= 0; i-- {
		switch e := entries[i].(type) {
		case AssistantEntry:
			return strings.TrimSpace(e.Content) != ""
		case CommandEntry:
			return strings.TrimSpace(e.Content) != ""
		case UserEntry:
			return false
		}
	}
	return false
}

func (m Model) handleSuggestionMsg(msg suggestionMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.activeSuggestion {
		return m, nil
	}
	m.activeSuggestion = 0
	if msg.err != nil {
		m.suggestion = ""
		return m, nil
	}
	if shouldFilterSuggestion(msg.text) {
		m.suggestion = ""
		return m, nil
	}
	m.suggestion = msg.text
	return m, nil
}

func buildSuggestionPrompt(entries []ThreadEntry) string {
	const (
		maxRecentItems = 8
		maxItemChars   = 240
	)

	original := ""
	var recent []string

	for _, entry := range entries {
		switch e := entry.(type) {
		case UserEntry:
			text := sanitizeSuggestionText(e.Content, maxItemChars)
			if text == "" {
				continue
			}
			if original == "" {
				original = text
			}
			recent = append(recent, "User: "+text)
		case AssistantEntry:
			text := sanitizeSuggestionText(e.Content, maxItemChars)
			if text == "" {
				continue
			}
			recent = append(recent, "Assistant: "+text)
		case CommandEntry:
			text := sanitizeSuggestionText(e.Content, maxItemChars)
			if text == "" {
				continue
			}
			recent = append(recent, "Assistant: $ "+text)
		}
	}

	if len(recent) == 0 {
		return ""
	}
	if len(recent) > maxRecentItems {
		recent = recent[len(recent)-maxRecentItems:]
	}

	var b strings.Builder
	if original != "" {
		b.WriteString("Original user request:\n")
		b.WriteString(original)
		b.WriteString("\n\n")
	}
	b.WriteString("Recent conversation:\n")
	b.WriteString(strings.Join(recent, "\n"))
	return b.String()
}

func sanitizeSuggestionText(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= maxChars {
		return text
	}
	return strings.TrimSpace(text[:maxChars-3]) + "..."
}

func shouldFilterSuggestion(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}

	lower := strings.ToLower(s)
	wordCount := len(strings.Fields(s))

	if strings.HasPrefix(lower, "nothing to suggest") || strings.HasPrefix(lower, "no suggestion") {
		return true
	}
	if suggestionWrappedMetaPattern.MatchString(s) {
		return true
	}
	if wordCount < 2 && !allowedSingleWordSuggestion(lower) && !strings.HasPrefix(s, "/") {
		return true
	}
	if wordCount > 12 || len(s) >= 100 {
		return true
	}
	if suggestionMultiSentencePattern.MatchString(s) {
		return true
	}
	if strings.ContainsAny(s, "\n*") || strings.Contains(s, "**") {
		return true
	}
	if suggestionEvaluativePattern.MatchString(lower) {
		return true
	}
	if suggestionAssistantVoicePattern.MatchString(s) {
		return true
	}
	if strings.HasSuffix(s, "?") {
		return true
	}
	return false
}

func allowedSingleWordSuggestion(lower string) bool {
	switch lower {
	case "yes", "yeah", "yep", "yea", "yup", "sure", "ok", "okay",
		"push", "commit", "deploy", "stop", "continue", "check", "exit", "quit", "no":
		return true
	default:
		return false
	}
}

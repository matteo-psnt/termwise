package agentui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/provider"
)

func TestTrimContextDropsOldToolResultsFirst(t *testing.T) {
	large := strings.Repeat("x", 2_000)
	messages := []provider.Message{
		{
			Role: "user",
			ToolResults: []provider.ToolResult{
				{ToolCallID: "1", Content: large},
				{ToolCallID: "2", Content: large},
			},
		},
		{
			Role:    "assistant",
			Content: "keep assistant text",
		},
	}

	trimmed := trimContext(messages, 2_000)
	if got := trimmed[0].ToolResults[0].Content; !strings.Contains(got, "result dropped") {
		t.Fatalf("expected oldest tool result to be dropped, got %q", got)
	}
	if got := trimmed[0].ToolResults[1].Content; strings.Contains(got, "result dropped") {
		t.Fatalf("expected trimming to stop after the oldest result, got %q", got)
	}
	if trimmed[1].Content != "keep assistant text" {
		t.Fatalf("expected assistant text to be preserved, got %q", trimmed[1].Content)
	}
}

func TestTrimContextLeavesMessagesWithinLimitUntouched(t *testing.T) {
	messages := []provider.Message{
		{Role: "user", Content: "short"},
		{Role: "assistant", Content: "also short"},
	}

	trimmed := trimContext(messages, 10_000)
	if trimmed[0].Content != "short" || trimmed[1].Content != "also short" {
		t.Fatalf("expected messages to remain unchanged, got %#v", trimmed)
	}
}

func TestHandleThinkingKeyEscInterruptsRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := Model{
		sharedState: sharedState{
			ctx:              ctx,
			cancel:           cancel,
			activeGeneration: 7,
			vp:               viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
		},
		state: stateThinking,
	}

	gotModel, _ := thinkingMode{}.handleKey(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	got := gotModel.(Model)

	if got.state != stateIdle {
		t.Fatalf("expected stateIdle after interrupt, got %v", got.state)
	}
	if got.activeGeneration != 0 {
		t.Fatalf("expected active generation to be cleared, got %d", got.activeGeneration)
	}
	if len(got.thread) != 1 {
		t.Fatalf("expected one thread entry, got %d", len(got.thread))
	}
	if entry, ok := got.thread[0].(ErrorEntry); !ok || entry.Content != "Request interrupted." {
		t.Fatalf("expected interrupt entry, got %#v", got.thread[0])
	}

	select {
	case <-ctx.Done():
	default:
		t.Fatal("expected the original context to be canceled")
	}
}

func TestHandleResponseEventIgnoresStaleResponse(t *testing.T) {
	m := Model{
		sharedState: sharedState{
			activeGeneration: 2,
			vp:               viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
		},
		state: stateIdle,
	}

	gotModel, _ := m.handleGenerationMsg(generationMsg{
		generation: 1,
		msg:        responseEvent{Resp: &provider.ChatResponse{Content: "stale"}},
	})
	got := gotModel.(Model)

	if len(got.thread) != 0 {
		t.Fatalf("expected no error thread entry, got %#v", got.thread)
	}
	if got.activeGeneration != 2 {
		t.Fatalf("expected active generation to remain unchanged, got %d", got.activeGeneration)
	}
}

func TestHandleResponseEventIgnoresCanceledActiveRequest(t *testing.T) {
	m := Model{
		sharedState: sharedState{
			activeGeneration: 3,
			vp:               viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
		},
		state: stateThinking,
	}

	gotModel, _ := m.handleGenerationMsg(generationMsg{
		generation: 3,
		msg:        responseEvent{Err: context.Canceled},
	})
	got := gotModel.(Model)

	if got.activeGeneration != 0 {
		t.Fatalf("expected active generation to be cleared, got %d", got.activeGeneration)
	}
	if len(got.thread) != 0 {
		t.Fatalf("expected no thread entry, got %#v", got.thread)
	}
}

func TestHandleToolExecutedEventIgnoresStaleGeneration(t *testing.T) {
	m := Model{
		sharedState: sharedState{
			activeGeneration: 2,
			vp:               viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
		},
	}

	gotModel, _ := m.handleGenerationMsg(generationMsg{
		generation: 1,
		msg: toolExecutedEvent{
			ToolCall: provider.ToolCall{Name: "bash", Input: map[string]any{"command": "echo stale"}},
			Result:   provider.ToolResult{ToolCallID: "1", Content: "stale"},
		},
	})
	got := gotModel.(Model)

	if len(got.thread) != 0 {
		t.Fatalf("expected stale tool result to be ignored, got %#v", got.thread)
	}
}

func TestHandleInitialPromptSubmitsPrompt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	suggestionCtx, suggestionCancel := context.WithCancel(context.Background())
	defer suggestionCancel()

	m := Model{
		sharedState: sharedState{
			ctx:              ctx,
			cancel:           cancel,
			suggestionCtx:    suggestionCtx,
			suggestionCancel: suggestionCancel,
			vp:               viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
			initialPrompt:    "inspect the repo",
			contextWindow:    32_000,
		},
		state: stateIdle,
	}

	gotModel, _ := m.handleInitialPrompt("inspect the repo")
	got := gotModel.(Model)

	if got.state != stateThinking {
		t.Fatalf("expected stateThinking after initial prompt, got %v", got.state)
	}
	if got.initialPrompt != "" {
		t.Fatalf("expected initialPrompt to be cleared, got %q", got.initialPrompt)
	}
	if len(got.thread) != 1 {
		t.Fatalf("expected one thread entry, got %d", len(got.thread))
	}
	if entry, ok := got.thread[0].(UserEntry); !ok || entry.Content != "inspect the repo" {
		t.Fatalf("expected user entry with prefill, got %#v", got.thread[0])
	}
	if len(got.messages) != 1 || got.messages[0].Content != "inspect the repo" {
		t.Fatalf("expected one user message, got %#v", got.messages)
	}
}

func TestHandleIdleKeyTabAcceptsSuggestion(t *testing.T) {
	m := Model{
		sharedState: sharedState{
			suggestion: "run the tests",
			input:      textarea.New(),
			vp:         viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
		},
		state: stateIdle,
	}

	gotModel, _ := idleMode{}.handleKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	got := gotModel.(Model)

	if got.input.Value() != "run the tests" {
		t.Fatalf("expected suggestion to be inserted, got %q", got.input.Value())
	}
	if got.suggestion != "" {
		t.Fatalf("expected suggestion to be cleared after accept, got %q", got.suggestion)
	}
}

func TestBuildSuggestionPromptIncludesOriginalRequestAndCommand(t *testing.T) {
	prompt := buildSuggestionPrompt([]ThreadEntry{
		UserEntry{Content: "fix the flaky test"},
		AssistantEntry{Content: "I found the race and updated the test."},
		CommandEntry{Content: "go test ./..."},
	})

	if !strings.Contains(prompt, "Original user request:\nfix the flaky test") {
		t.Fatalf("expected original request in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "Assistant: $ go test ./...") {
		t.Fatalf("expected command entry in prompt, got %q", prompt)
	}
}

func TestShouldFilterSuggestion(t *testing.T) {
	tests := []struct {
		suggestion string
		filtered   bool
	}{
		{suggestion: "run the tests", filtered: false},
		{suggestion: "Looks good", filtered: true},
		{suggestion: "What about logs?", filtered: true},
		{suggestion: "yes", filtered: false},
	}

	for _, tt := range tests {
		if got := shouldFilterSuggestion(tt.suggestion); got != tt.filtered {
			t.Fatalf("shouldFilterSuggestion(%q) = %v, want %v", tt.suggestion, got, tt.filtered)
		}
	}
}

func TestHandleRespondEventStoresShellCommand(t *testing.T) {
	m := Model{
		sharedState: sharedState{
			vp: viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
		},
		state: stateThinking,
	}

	gotModel, _ := m.handleRespondEvent(respondEvent{
		Content: "go test ./...",
	})
	got := gotModel.(Model)

	if got.shellCommand != "go test ./..." {
		t.Fatalf("expected shell command result, got %q", got.shellCommand)
	}
}

func TestSubmitMessageClearsPendingShellCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	suggestionCtx, suggestionCancel := context.WithCancel(context.Background())
	defer suggestionCancel()

	m := Model{
		sharedState: sharedState{
			ctx:              ctx,
			cancel:           cancel,
			suggestionCtx:    suggestionCtx,
			suggestionCancel: suggestionCancel,
			vp:               viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
			contextWindow:    32_000,
			shellCommand:     "go test ./...",
		},
		state: stateIdle,
	}

	gotModel, _ := m.submitMessage("next question")
	got := gotModel.(Model)

	if got.exitShellCommand() != "" {
		t.Fatalf("expected shell result to clear on new prompt, got %q", got.shellCommand)
	}
}

func TestHandleResponseEventWithoutToolsClearsShellCommand(t *testing.T) {
	m := Model{
		sharedState: sharedState{
			vp:           viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
			shellCommand: "go test ./...",
		},
		state: stateThinking,
	}

	gotModel, _ := m.handleResponseEvent(responseEvent{
		Resp: &provider.ChatResponse{Content: "plain text"},
	})
	got := gotModel.(Model)

	if got.exitShellCommand() != "" {
		t.Fatalf("expected shell result to clear after plain text response, got %q", got.shellCommand)
	}
}

// Truncation counts and cuts runes, so a multi-byte character at the limit is
// never split into invalid UTF-8.
func TestSanitizeSuggestionTextIsRuneSafe(t *testing.T) {
	text := strings.Repeat("é", 20)
	got := sanitizeSuggestionText(text, 10)
	if want := strings.Repeat("é", 7) + "..."; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := sanitizeSuggestionText(strings.Repeat("é", 10), 10); got != strings.Repeat("é", 10) {
		t.Fatalf("text within the limit was truncated: %q", got)
	}
}

// trimContext returns a trimmed copy for the request; the caller's messages are
// the session history and must keep the full tool results.
func TestTrimContextDoesNotMutateInput(t *testing.T) {
	large := strings.Repeat("x", 2_000)
	messages := []provider.Message{{
		Role: "user",
		ToolResults: []provider.ToolResult{
			{ToolCallID: "1", Content: large},
			{ToolCallID: "2", Content: large},
		},
	}}

	trimmed := trimContext(messages, 2_000)
	if !strings.Contains(trimmed[0].ToolResults[0].Content, "result dropped") {
		t.Fatalf("expected the trimmed copy to drop a result")
	}
	for i, tr := range messages[0].ToolResults {
		if tr.Content != large {
			t.Fatalf("input ToolResults[%d] was rewritten to %q", i, tr.Content)
		}
	}
}

// Ctrl+C and the close key abort: a proposal the user never accepted must not
// reach the shell widget. Enter on the proposal still does.
func TestQuitKeysDoNotAcceptPendingProposal(t *testing.T) {
	proposal := func(state tuiState) Model {
		ctx, cancel := context.WithCancel(context.Background())
		sctx, scancel := context.WithCancel(context.Background())
		m := pickerModel("")
		m.ctx, m.cancel = ctx, cancel
		m.suggestionCtx, m.suggestionCancel = sctx, scancel
		m.closeKey = "^T"
		m.shellCommand = "rm -rf build"
		m.state = state
		return m
	}
	for name, key := range map[string]tea.KeyPressMsg{
		"ctrl+c":    {Code: 'c', Mod: tea.ModCtrl},
		"close key": {Code: 't', Mod: tea.ModCtrl},
	} {
		for _, state := range []tuiState{stateCommandProposal, stateCommandProposalEdit} {
			next, _ := proposal(state).handleKey(key)
			got := next.(Model)
			if !got.quitting {
				t.Fatalf("%s in state %v did not quit", name, state)
			}
			if cmd := got.exitShellCommand(); cmd != "" {
				t.Errorf("%s in state %v handed %q to the shell", name, state, cmd)
			}
		}
	}

	next, _ := proposal(stateCommandProposal).handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd := next.(Model).exitShellCommand(); cmd != "rm -rf build" {
		t.Fatalf("enter handed %q to the shell, want the proposal", cmd)
	}
}

// "?" is text, not the help key, wherever the user is typing.
func TestQuestionMarkIsTextWhereTheUserIsTyping(t *testing.T) {
	question := tea.KeyPressMsg{Code: '?', Text: "?"}

	edit := pickerModel("")
	edit.state = stateCommandProposalEdit
	edit.input.Focus()
	edit.input.SetValue("ls file")
	next, _ := edit.handleKey(question)
	if got := next.(Model); got.showHelp || got.input.Value() != "ls file?" {
		t.Errorf("edit mode: help=%v input=%q, want the ? typed", got.showHelp, got.input.Value())
	}

	search := pickerModel("")
	search.state = stateHistSearch
	next, _ = search.handleKey(question)
	if got := next.(Model); got.showHelp || got.histSearch.query != "?" {
		t.Errorf("hist search: help=%v query=%q, want the ? typed", got.showHelp, got.histSearch.query)
	}

	ask := pickerModel("")
	ask.state = stateAskPicker
	p := newPicker("Which?", nil, false) // no options: the cursor starts on Other
	ask.pending.picker = &p
	next, _ = ask.handleKey(question)
	if got := next.(Model); got.showHelp || got.pending.picker.otherText != "?" {
		t.Errorf("ask Other: help=%v text=%q, want the ? typed", got.showHelp, got.pending.picker.otherText)
	}

	idle := pickerModel("")
	next, _ = idle.handleKey(question)
	if !next.(Model).showHelp {
		t.Error("? on an empty idle prompt should still open help")
	}
}

func toolResultIDs(msg provider.Message) []string {
	var ids []string
	for _, tr := range msg.ToolResults {
		ids = append(ids, tr.ToolCallID)
	}
	return ids
}

// A turn interrupted after the model asked for tools leaves calls with no
// results; the next message must answer them or the provider rejects it.
func TestSubmitAfterInterruptAnswersDanglingToolCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := Model{
		sharedState: sharedState{
			ctx:              ctx,
			cancel:           cancel,
			suggestionCtx:    ctx,
			suggestionCancel: func() {},
			activeGeneration: 3,
			vp:               viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
			messages: []provider.Message{
				{Role: "user", Content: "list files"},
				{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "a", Name: "bash"}, {ID: "b", Name: "read"}}},
			},
		},
		state: stateThinking,
	}
	next, _ := thinkingMode{}.handleKey(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	next, _ = next.(Model).submitMessage("never mind")
	msgs := next.(Model).messages

	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want prompt, calls, results, new prompt: %#v", len(msgs), msgs)
	}
	if ids := toolResultIDs(msgs[2]); len(ids) != 2 || ids[0] != "a" || ids[1] != "b" || !msgs[2].ToolResults[0].IsError {
		t.Fatalf("results message = %#v, want error results for a and b", msgs[2])
	}
	if msgs[3].Content != "never mind" {
		t.Fatalf("last message = %#v, want the new prompt", msgs[3])
	}
}

// Calls answered only in part (the command tool ends the turn before the
// calls after it) get the missing results added to the existing message.
func TestCloseDanglingToolCallsCompletesPartialResults(t *testing.T) {
	results := []provider.ToolResult{{ToolCallID: "cmd", Content: "ok"}}
	in := []provider.Message{
		{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "cmd"}, {ID: "x"}}},
		{Role: "user", ToolResults: results},
		{Role: "assistant", Content: "done"},
	}
	out := closeDanglingToolCalls(in)
	if len(out) != 3 {
		t.Fatalf("got %d messages, want 3", len(out))
	}
	if ids := toolResultIDs(out[1]); len(ids) != 2 || ids[0] != "cmd" || ids[1] != "x" {
		t.Fatalf("results = %v, want cmd and x", ids)
	}
	if len(in[1].ToolResults) != 1 || len(results) != 1 {
		t.Fatal("input was modified")
	}

	complete := closeDanglingToolCalls(out)
	if len(complete) != 3 || len(complete[1].ToolResults) != 2 {
		t.Fatalf("a complete history changed: %#v", complete)
	}
}

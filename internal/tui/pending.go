package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/ai"
	"github.com/matteo-psnt/termwise/internal/allowlist"
)

type pendingToolState struct {
	toolCall  ai.ToolCall
	remaining []ai.ToolCall
	collected []ai.ToolResult
	picker    *picker
}

func newPendingToolState(tc ai.ToolCall, remaining []ai.ToolCall, collected []ai.ToolResult) pendingToolState {
	return pendingToolState{
		toolCall:  tc,
		remaining: remaining,
		collected: collected,
	}
}

func (p pendingToolState) withPicker(picker picker) pendingToolState {
	p.picker = &picker
	return p
}

func (p pendingToolState) result(content string, isError bool) ai.ToolResult {
	return ai.ToolResult{
		ToolCallID: p.toolCall.ID,
		Content:    content,
		IsError:    isError,
	}
}

func (m *Model) appendThreadEntries(entries ...ThreadEntry) {
	m.thread = append(m.thread, entries...)
}

func (m *Model) setPendingTool(tc ai.ToolCall, remaining []ai.ToolCall, collected []ai.ToolResult) {
	m.pending = newPendingToolState(tc, remaining, collected)
}

func (m *Model) clearPendingTool() {
	m.pending = pendingToolState{}
}

func (m *Model) appendToolResultsMessage(collected []ai.ToolResult) {
	m.messages = append(m.messages, ai.Message{
		Role:        "user",
		ToolResults: collected,
	})
}

func (m *Model) interruptThinking() {
	m.cancel()
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel
	m.state = stateIdle
	m.appendThreadEntries(ThreadEntry{Kind: EntryError, Content: "interrupted"})
	m.refreshViewport()
}

func (m Model) approvePendingBash(saveRule bool) (tea.Model, tea.Cmd) {
	if saveRule {
		cmd, _ := m.pending.toolCall.Input["command"].(string)
		rule := allowlist.BuildRuleFromCommand(cmd)
		if rule != "" {
			m.allowRules = append(m.allowRules, rule)
			m.saveAllowRules()
		}
	}

	tc := m.pending.toolCall
	remaining := m.pending.remaining
	collected := m.pending.collected
	m.clearPendingTool()
	m.state = stateThinking
	return m, tea.Batch(
		m.spin.Tick,
		agent.ExecuteBashCmd(tc, remaining, collected),
	)
}

func (m Model) resumePendingToolLoop(result ai.ToolResult) (tea.Model, tea.Cmd) {
	remaining := m.pending.remaining
	collected := append(m.pending.collected, result)
	m.clearPendingTool()
	m.state = stateThinking
	return m, tea.Batch(
		m.spin.Tick,
		agent.ProcessToolsCmd(remaining, collected, m.needsApproval),
	)
}

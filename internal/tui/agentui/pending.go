package agentui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/provider"
)

type pendingToolState struct {
	toolCall  provider.ToolCall
	remaining []provider.ToolCall
	collected []provider.ToolResult
	picker    *picker
}

func newPendingToolState(tc provider.ToolCall, remaining []provider.ToolCall, collected []provider.ToolResult) pendingToolState {
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

func (p pendingToolState) result(content string, isError bool) provider.ToolResult {
	return provider.ToolResult{
		ToolCallID: p.toolCall.ID,
		Content:    content,
		IsError:    isError,
	}
}

func (m *Model) appendThreadEntries(entries ...ThreadEntry) {
	m.thread = append(m.thread, entries...)
}

func (m *Model) setPendingTool(tc provider.ToolCall, remaining []provider.ToolCall, collected []provider.ToolResult) {
	m.pending = newPendingToolState(tc, remaining, collected)
}

func (m *Model) clearPendingTool() {
	m.pending = pendingToolState{}
}

func (m *Model) appendToolResultsMessage(collected []provider.ToolResult) {
	m.messages = append(m.messages, provider.Message{
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
	m.appendThreadEntries(ErrorEntry{Content: "interrupted"})
	m.refreshViewport()
}

func (m Model) approvePendingBash() (tea.Model, tea.Cmd) {
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

func (m Model) resumePendingToolLoop(result provider.ToolResult) (tea.Model, tea.Cmd) {
	remaining := m.pending.remaining
	collected := append(m.pending.collected, result)
	m.clearPendingTool()
	m.state = stateThinking
	return m, tea.Batch(
		m.spin.Tick,
		agent.ProcessToolsCmd(remaining, collected, m.needsApproval),
	)
}

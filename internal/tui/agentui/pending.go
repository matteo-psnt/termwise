package agentui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/provider"
)

type pendingToolState struct {
	toolCall  provider.ToolCall
	remaining []provider.ToolCall
	collected []provider.ToolResult
	picker    *picker
}

func (p pendingToolState) result(content string, isError bool) provider.ToolResult {
	return provider.ToolResult{
		ToolCallID: p.toolCall.ID,
		Content:    content,
		IsError:    isError,
	}
}

func (m *Model) setPendingTool(tc provider.ToolCall, remaining []provider.ToolCall, collected []provider.ToolResult) {
	m.turnResults = collected
	m.pending = pendingToolState{
		toolCall:  tc,
		remaining: remaining,
		collected: collected,
	}
}

func (m *Model) clearPendingTool() {
	m.pending = pendingToolState{}
}

func (m *Model) appendToolResultsMessage(collected []provider.ToolResult) {
	m.turnResults = nil
	m.messages = append(m.messages, provider.Message{
		Role:        "user",
		ToolResults: collected,
	})
}

// flushTurnResults records the results of tool calls that already ran when
// a turn ends early (Esc, quitting at an approval prompt). Without this they
// were dropped, and closeDanglingToolCalls then told the model every call of
// the turn had not run, inviting it to repeat a command that had already
// changed something. Calls that really did not finish are still answered
// there.
func (m *Model) flushTurnResults() {
	if len(m.turnResults) == 0 {
		return
	}
	m.appendToolResultsMessage(append([]provider.ToolResult(nil), m.turnResults...))
}

func (m Model) approvePendingBash() (tea.Model, tea.Cmd) {
	tc := m.pending.toolCall
	remaining := m.pending.remaining
	collected := m.pending.collected
	m.clearPendingTool()
	m.state = stateThinking
	return m, tea.Batch(
		m.spin.Tick,
		m.wrapActiveGeneration(executeBashCmd(m.ctx, tc, remaining, collected)),
	)
}

func (m Model) resumePendingToolLoop(result provider.ToolResult) (tea.Model, tea.Cmd) {
	remaining := m.pending.remaining
	//nolint:gocritic // intentional: snapshot m.pending.collected + append result
	collected := append(m.pending.collected, result)
	m.clearPendingTool()
	m.state = stateThinking
	return m, tea.Batch(
		m.spin.Tick,
		m.wrapActiveGeneration(processToolsCmd(m.ctx, remaining, collected)),
	)
}

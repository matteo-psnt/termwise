package agentui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/matteo-psnt/termwise/internal/agent"
)

// generationMsg wraps a downstream message with the generation token it
// belongs to. Stale messages (with a different generation than the active one)
// are dropped, so an interrupted turn cannot resurrect itself.
type generationMsg struct {
	generation uint64
	msg        tea.Msg
}

func newGenerationContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func newSuggestionContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 8*time.Second)
}

// wrapGenerationCmd tags every message produced by cmd with the given
// generation token. nil msgs and nil cmds are passed through unchanged.
func wrapGenerationCmd(generation uint64, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if msg == nil {
			return nil
		}
		return generationMsg{generation: generation, msg: msg}
	}
}

func (m *Model) beginGeneration() {
	m.resetGenerationContext()
	m.nextGeneration++
	m.activeGeneration = m.nextGeneration
}

func (m *Model) resetGenerationContext() {
	m.cancel()
	m.ctx, m.cancel = newGenerationContext()
}

func (m *Model) startChat() tea.Cmd {
	return tea.Batch(
		m.spin.Tick,
		m.wrapActiveGeneration(ChatCmd(m.ctx, m.provider, m.chatRequest())),
	)
}

func (m *Model) interruptActiveTurn() {
	m.resetGenerationContext()
	m.finishGeneration()
	m.clearPendingTool()
	m.state = stateIdle
	m.appendThreadEntries(ErrorEntry{Content: "Request interrupted."})
	m.refreshViewport()
}

func (m *Model) finishGeneration() {
	m.activeGeneration = 0
}

func (m Model) wrapActiveGeneration(cmd tea.Cmd) tea.Cmd {
	return wrapGenerationCmd(m.activeGeneration, cmd)
}

func (m Model) handleGenerationMsg(msg generationMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.activeGeneration {
		return m, nil
	}

	switch inner := msg.msg.(type) {
	case agent.ResponseEvent:
		return m.handleResponseEvent(inner)
	case agent.ToolExecutedEvent:
		return m.handleToolExecutedEvent(inner)
	case agent.NeedsApprovalEvent:
		return m.handleNeedsApprovalEvent(inner)
	case agent.AskEvent:
		return m.handleAskEvent(inner)
	case agent.RespondEvent:
		return m.handleRespondEvent(inner)
	case agent.AllToolsDoneEvent:
		return m.handleAllToolsDoneEvent(inner)
	case judgmentMsg:
		return m.handleJudgmentMsg(inner)
	default:
		return m, nil
	}
}

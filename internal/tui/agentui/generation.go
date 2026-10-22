package agentui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// generationMsg wraps a downstream message with the generation token it
// belongs to. Stale messages (with a different generation than the active one)
// are dropped, so an interrupted turn cannot resurrect itself.
type generationMsg struct {
	generation uint64
	msg        tea.Msg
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
	m.turnStartedAt = time.Now()
	m.activity = ""
}

// setActivity records what the agent is about to do, so the thinking row can
// say "searching for handleKey" instead of "thinking...". A tool with no
// registered detail formatter falls back to its bare name.
func (m *Model) setActivity(tc provider.ToolCall) {
	if detail := tools.Detail(tc); detail != "" {
		m.activity = activityVerb(tc.Name) + " " + detail
		return
	}
	m.activity = activityVerb(tc.Name)
}

// activityVerb reads as a present participle in the status row.
func activityVerb(tool string) string {
	switch tool {
	case "read":
		return "reading"
	case "bash":
		return "running"
	case "grep":
		return "searching for"
	case "glob":
		return "looking for"
	case "command":
		return "preparing command"
	case "ask":
		return "asking"
	default:
		return tool
	}
}

func (m *Model) resetGenerationContext() {
	m.cancel()
	m.ctx, m.cancel = context.WithCancel(context.Background())
}

func (m *Model) startChat() tea.Cmd {
	return tea.Batch(
		m.spin.Tick,
		m.wrapActiveGeneration(chatCmd(m.ctx, m.provider, m.chatRequest())),
	)
}

func (m *Model) interruptActiveTurn() {
	m.resetGenerationContext()
	m.finishGeneration()
	m.flushTurnResults()
	m.clearPendingTool()
	m.state = stateIdle
	m.appendThreadEntries(ErrorEntry{Content: "Request interrupted."})
	m.refreshViewport()
}

func (m *Model) finishGeneration() {
	m.activeGeneration = 0
	m.activity = ""
}

func (m Model) wrapActiveGeneration(cmd tea.Cmd) tea.Cmd {
	return wrapGenerationCmd(m.activeGeneration, cmd)
}

func (m Model) handleGenerationMsg(msg generationMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.activeGeneration {
		return m, nil
	}

	switch inner := msg.msg.(type) {
	case responseEvent:
		return m.handleResponseEvent(inner)
	case toolExecutedEvent:
		return m.handleToolExecutedEvent(inner)
	case needsApprovalEvent:
		return m.handleNeedsApprovalEvent(inner)
	case askEvent:
		return m.handleAskEvent(inner)
	case respondEvent:
		return m.handleRespondEvent(inner)
	case allToolsDoneEvent:
		return m.handleAllToolsDoneEvent(inner)
	case judgmentMsg:
		return m.handleJudgmentMsg(inner)
	default:
		return m, nil
	}
}

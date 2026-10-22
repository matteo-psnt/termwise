package agentui

import (
	"context"
	"errors"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/provider"
)

func (m Model) handleSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if !m.isBusy() {
		return m, nil
	}
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

func (m Model) handleResponseEvent(msg responseEvent) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.finishGeneration()
		if errors.Is(msg.Err, context.Canceled) {
			return m, nil
		}
		m.state = stateIdle
		m.appendThreadEntries(ErrorEntry{Content: msg.Err.Error()})
		m.refreshViewport()
		return m, nil
	}

	resp := msg.Resp
	m.inputTokens += resp.InputTokens
	m.outputTokens += resp.OutputTokens
	m.messages = append(m.messages, provider.Message{
		Role:      "assistant",
		Content:   resp.Content,
		ToolCalls: resp.ToolCalls,
	})

	if resp.Content != "" {
		m.appendThreadEntries(AssistantEntry{Content: resp.Content})
	}

	if len(resp.ToolCalls) == 0 {
		m.logExchange(history.ExchangeKindText, resp.Content)
		m.clearShellCommand()
		m.finishGeneration()
		m.state = stateIdle
		m.refreshViewport()
		m.saveSession()
		return m, m.startSuggestion()
	}

	m.setActivity(resp.ToolCalls[0])
	m.refreshViewport()
	return m, m.wrapActiveGeneration(processToolsCmd(m.ctx, resp.ToolCalls, nil))
}

func (m Model) handleToolExecutedEvent(msg toolExecutedEvent) (tea.Model, tea.Cmd) {
	display := tools.DisplayResult(msg.ToolCall.Name, msg.Result.Content)

	// A gated call was already announced by handleNeedsApprovalEvent — whether
	// the user approved it or the judge did. Re-announcing it here printed the
	// same command twice with one result hanging off the pair.
	if msg.AutoAccepted {
		m.appendThreadEntries(ToolCallEntry{Name: msg.ToolCall.Name, Detail: tools.Detail(msg.ToolCall)})
	}
	m.appendThreadEntries(
		ToolResultEntry{Content: display, IsError: msg.Result.IsError},
	)
	m.refreshViewport()
	return m, m.wrapActiveGeneration(processToolsCmd(m.ctx, msg.Remaining, msg.Collected))
}

func (m Model) handleNeedsApprovalEvent(msg needsApprovalEvent) (tea.Model, tea.Cmd) {
	m.setPendingTool(msg.ToolCall, msg.Remaining, msg.Collected)
	m.appendThreadEntries(ToolCallEntry{Name: msg.ToolCall.Name, Detail: msg.Command})
	m.refreshViewport()

	if m.llmJudge {
		m.state = stateJudging
		return m, tea.Batch(m.spin.Tick, m.wrapActiveGeneration(m.judgeCmd(msg.ToolCall)))
	}

	m.state = stateApproval
	return m, nil
}

func (m Model) handleJudgmentMsg(msg judgmentMsg) (tea.Model, tea.Cmd) {
	if msg.safe {
		return m.approvePendingBash()
	}
	m.state = stateApproval
	return m, nil
}

func (m Model) handleAskEvent(msg askEvent) (tea.Model, tea.Cmd) {
	m.state = stateAskPicker
	m.setPendingTool(msg.ToolCall, msg.Remaining, msg.Collected)
	p := newPicker(msg.Question, msg.Options, msg.MultiSelect)
	m.pending.picker = &p
	if msg.Question != "" {
		m.appendThreadEntries(AssistantEntry{Content: msg.Question})
	}
	m.refreshViewport()
	return m, nil
}

func (m Model) handleRespondEvent(msg respondEvent) (tea.Model, tea.Cmd) {
	m.logExchange(history.ExchangeKindCommand, msg.Content)
	m.setShellCommand(msg.Content)
	m.appendThreadEntries(CommandEntry{Content: msg.Content})
	m.appendToolResultsMessage(msg.Collected)
	m.finishGeneration()
	m.state = stateCommandProposal
	m.refreshViewport()
	m.saveSession()
	return m, nil
}

func (m Model) handleAllToolsDoneEvent(msg allToolsDoneEvent) (tea.Model, tea.Cmd) {
	m.appendToolResultsMessage(msg.Collected)
	m.state = stateThinking
	return m, m.startChat()
}

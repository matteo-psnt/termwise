package agentui

import (
	"context"
	"errors"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/allowlist"
	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/theme"
)

type judgmentMsg struct {
	safe bool
}

func (m Model) judgeCmd(tc provider.ToolCall) tea.Cmd {
	cmd := tools.BashCommand(tc)
	client := m.provider
	modelID := m.modelID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		safe, err := agent.JudgeBashCommand(ctx, client, modelID, cmd)
		if err != nil {
			return judgmentMsg{safe: false}
		}
		return judgmentMsg{safe: safe}
	}
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	vpW, vpH := m.viewportDims()
	if !m.ready {
		m.vp = viewport.New(viewport.WithWidth(vpW), viewport.WithHeight(vpH))
		m.ready = true
	} else {
		m.vp.SetWidth(vpW)
		m.vp.SetHeight(vpH)
	}
	m.input.SetWidth(vpW - 4)
	// glamour fixes the wrap width when the renderer is built, so a resize needs
	// a new one or the thread keeps wrapping to the old terminal.
	m.renderer = newRenderer(m.hasDarkBg, theme.Get(m.themeName), m.renderWidth())
	m.refreshViewport()
	return m, nil
}

// renderWidth is the column budget for rendered markdown: the viewport, less
// the gutter the thread indents entries by. Zero before the first
// WindowSizeMsg, which glamour reads as "do not wrap yet".
func (m Model) renderWidth() int {
	vpW, _ := m.viewportDims()
	if vpW <= threadGutter {
		return 0
	}
	return vpW - threadGutter
}

func (m Model) handleSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if m.state != stateThinking && m.state != stateJudging {
		return m, nil
	}
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

func (m Model) handleResponseEvent(msg agent.ResponseEvent) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.finishGeneration()
		if errors.Is(msg.Err, context.Canceled) {
			return m, nil
		}
		m.state = stateIdle
		m.appendThreadEntries(ErrorEntry{Content: providerErrMsg(msg.Err)})
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
	return m, m.wrapActiveGeneration(ProcessToolsCmd(m.ctx, resp.ToolCalls, nil, allowlist.NeedsApproval))
}

func (m Model) handleToolExecutedEvent(msg agent.ToolExecutedEvent) (tea.Model, tea.Cmd) {
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
	return m, m.wrapActiveGeneration(ProcessToolsCmd(m.ctx, msg.Remaining, msg.Collected, allowlist.NeedsApproval))
}

func (m Model) handleNeedsApprovalEvent(msg agent.NeedsApprovalEvent) (tea.Model, tea.Cmd) {
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

func (m Model) handleAskEvent(msg agent.AskEvent) (tea.Model, tea.Cmd) {
	m.state = stateAskPicker
	m.setPendingTool(msg.ToolCall, msg.Remaining, msg.Collected)
	p := newPicker(msg.Question, msg.Options, msg.MultiSelect)
	m.pending = m.pending.withPicker(p)
	if msg.Question != "" {
		m.appendThreadEntries(AssistantEntry{Content: msg.Question})
	}
	m.refreshViewport()
	return m, nil
}

func (m Model) handleRespondEvent(msg agent.RespondEvent) (tea.Model, tea.Cmd) {
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

func (m Model) handleAllToolsDoneEvent(msg agent.AllToolsDoneEvent) (tea.Model, tea.Cmd) {
	m.appendToolResultsMessage(msg.Collected)
	m.state = stateThinking
	return m, m.startChat()
}

func (m Model) updateViewportOnly(msg tea.Msg) (tea.Model, tea.Cmd) {
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	if m.state != stateIdle && m.state != stateThinking && m.state != stateJudging && m.state != stateCommandProposal {
		return m, inputCmd
	}
	var vpCmd tea.Cmd
	m.vp, vpCmd = m.vp.Update(msg)
	return m, tea.Batch(vpCmd, inputCmd)
}

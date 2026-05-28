package agentui

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/provider"
)

type judgmentMsg struct {
	safe bool
}

func (m Model) judgeCmd(tc provider.ToolCall) tea.Cmd {
	cmd, _ := tc.Input["command"].(string)
	client := m.provider
	modelID := m.modelID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		resp, err := client.Chat(ctx, provider.ChatRequest{
			Model:  modelID,
			System: "You are a safety classifier for bash commands. Respond with exactly one word: safe or unsafe.",
			Messages: []provider.Message{
				{Role: "user", Content: "Should this bash command auto-execute without user confirmation?\n\n" + cmd},
			},
		})
		if err != nil {
			return judgmentMsg{safe: false}
		}
		return judgmentMsg{safe: strings.ToLower(strings.TrimSpace(resp.Content)) == "safe"}
	}
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	vpW, vpH := m.viewportDims()
	if !m.ready {
		m.vp = viewport.New(vpW, vpH)
		m.ready = true
	} else {
		m.vp.Width = vpW
		m.vp.Height = vpH
	}
	m.input.Width = vpW - 4
	m.refreshViewport()
	return m, nil
}

func (m Model) handleSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if m.state != stateThinking && m.state != stateJudging {
		return m, nil
	}
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

func (m Model) handleResponseMsg(msg agent.ResponseMsg) (tea.Model, tea.Cmd) {
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
		m.finishGeneration()
		m.state = stateIdle
		m.refreshViewport()
		m.saveSession()
		return m, nil
	}

	m.refreshViewport()
	return m, m.wrapActiveGeneration(agent.ProcessToolsCmd(m.ctx, resp.ToolCalls, nil, m.needsApproval))
}

func (m Model) handleToolExecutedMsg(msg agent.ToolExecutedMsg) (tea.Model, tea.Cmd) {
	display := msg.Result.Content
	if msg.ToolCall.Name == "bash" {
		display = tools.FormatDisplay(msg.Result.Content)
	}
	m.appendThreadEntries(
		ToolCallEntry{Name: msg.ToolCall.Name, Detail: toolDetail(msg.ToolCall)},
		ToolResultEntry{Content: display, IsError: msg.Result.IsError},
	)
	m.refreshViewport()
	return m, m.wrapActiveGeneration(agent.ProcessToolsCmd(m.ctx, msg.Remaining, msg.Collected, m.needsApproval))
}

func (m Model) handleNeedsApprovalMsg(msg agent.NeedsApprovalMsg) (tea.Model, tea.Cmd) {
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

func (m Model) handleAskMsg(msg agent.AskMsg) (tea.Model, tea.Cmd) {
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

func (m Model) handleRespondMsg(msg agent.RespondMsg) (tea.Model, tea.Cmd) {
	if msg.RespondType == "command" {
		m.appendThreadEntries(CommandEntry{Content: msg.Content})
	} else {
		m.appendThreadEntries(AssistantEntry{Content: msg.Content})
	}
	m.appendToolResultsMessage(msg.Collected)
	m.finishGeneration()
	m.state = stateIdle
	m.refreshViewport()
	m.saveSession()
	return m, nil
}

func (m Model) handleAllToolsDoneMsg(msg agent.AllToolsDoneMsg) (tea.Model, tea.Cmd) {
	m.appendToolResultsMessage(msg.Collected)
	m.state = stateThinking
	return m, m.startChat()
}

func (m Model) updateViewportOnly(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.state != stateIdle && m.state != stateThinking && m.state != stateJudging {
		return m, nil
	}
	var vpCmd tea.Cmd
	m.vp, vpCmd = m.vp.Update(msg)
	return m, vpCmd
}

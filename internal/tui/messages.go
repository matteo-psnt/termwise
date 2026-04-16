package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteo-psnt/termwise/internal/agent"
	"github.com/matteo-psnt/termwise/internal/ai"
)

type judgmentMsg struct{ safe bool }

func (m Model) judgeCmd(tc ai.ToolCall) tea.Cmd {
	cmd, _ := tc.Input["command"].(string)
	provider := m.provider
	modelID := m.modelID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resp, err := provider.Chat(ctx, ai.ChatRequest{
			Model:  modelID,
			System: "You are a safety classifier for bash commands. Respond with exactly one word: safe or unsafe.",
			Messages: []ai.Message{
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
		m.state = stateIdle
		m.appendThreadEntries(ThreadEntry{Kind: EntryError, Content: providerErrMsg(msg.Err)})
		m.refreshViewport()
		return m, nil
	}

	resp := msg.Resp
	m.inputTokens += resp.InputTokens
	m.outputTokens += resp.OutputTokens
	m.messages = append(m.messages, ai.Message{
		Role:      "assistant",
		Content:   resp.Content,
		ToolCalls: resp.ToolCalls,
	})

	if len(resp.ToolCalls) == 0 {
		if resp.Content != "" {
			m.appendThreadEntries(ThreadEntry{Kind: EntryAssistant, Content: resp.Content})
		}
		m.state = stateIdle
		m.refreshViewport()
		return m, nil
	}

	return m, agent.ProcessToolsCmd(resp.ToolCalls, nil, m.needsApproval)
}

func (m Model) handleToolExecutedMsg(msg agent.ToolExecutedMsg) (tea.Model, tea.Cmd) {
	m.appendThreadEntries(
		ThreadEntry{
			Kind:       EntryToolCall,
			ToolName:   msg.ToolCall.Name,
			ToolDetail: toolDetail(msg.ToolCall),
			Auto:       msg.AutoAccepted,
		},
		ThreadEntry{
			Kind:    EntryToolResult,
			Content: msg.Result.Content,
			IsError: msg.Result.IsError,
		},
	)
	m.refreshViewport()
	return m, agent.ProcessToolsCmd(msg.Remaining, msg.Collected, m.needsApproval)
}

func (m Model) handleNeedsApprovalMsg(msg agent.NeedsApprovalMsg) (tea.Model, tea.Cmd) {
	m.setPendingTool(msg.ToolCall, msg.Remaining, msg.Collected)
	m.appendThreadEntries(ThreadEntry{
		Kind:       EntryToolCall,
		ToolName:   msg.ToolCall.Name,
		ToolDetail: msg.Command,
		Auto:       false,
	})
	m.refreshViewport()

	if m.llmJudge {
		m.state = stateJudging
		return m, tea.Batch(m.spin.Tick, m.judgeCmd(msg.ToolCall))
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
		m.appendThreadEntries(ThreadEntry{Kind: EntryAssistant, Content: msg.Question})
	}
	m.refreshViewport()
	return m, nil
}

func (m Model) handleRespondMsg(msg agent.RespondMsg) (tea.Model, tea.Cmd) {
	m.appendThreadEntries(ThreadEntry{
		Kind:        EntryRespond,
		RespondType: msg.RespondType,
		Content:     msg.Content,
	})
	m.appendToolResultsMessage(msg.Collected)
	m.state = stateIdle
	m.refreshViewport()
	return m, nil
}

func (m Model) handleAllToolsDoneMsg(msg agent.AllToolsDoneMsg) (tea.Model, tea.Cmd) {
	m.appendToolResultsMessage(msg.Collected)
	m.state = stateThinking
	return m, tea.Batch(
		m.spin.Tick,
		agent.ChatCmd(m.ctx, m.provider, m.chatRequest()),
	)
}

func (m Model) updateViewportOnly(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.state != stateIdle && m.state != stateThinking && m.state != stateJudging {
		return m, nil
	}
	var vpCmd tea.Cmd
	m.vp, vpCmd = m.vp.Update(msg)
	return m, vpCmd
}

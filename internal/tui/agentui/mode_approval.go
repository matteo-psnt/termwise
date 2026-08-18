package agentui

import tea "github.com/charmbracelet/bubbletea"

// approvalMode is active when a bash command is awaiting user approval.
type approvalMode struct{}

func (approvalMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		return m.approvePendingBash()
	case tea.KeyEsc:
		return m.denyPendingBash()
	}
	switch msg.String() {
	case "1", "y":
		return m.approvePendingBash()
	case "2", "n":
		return m.denyPendingBash()
	case "3", "e":
		cmd, _ := m.pending.toolCall.Input["command"].(string)
		m.input.SetValue(cmd)
		m.input.CursorEnd()
		m.state = stateApprovalEdit
		return m, nil
	}
	return m, nil
}

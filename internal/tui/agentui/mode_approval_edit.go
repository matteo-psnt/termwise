package agentui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// approvalEditMode is active while the user is editing a bash command before
// approving it.
type approvalEditMode struct{}

func (approvalEditMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		edited := strings.TrimSpace(m.input.Value())
		m.input.SetValue("")
		if edited != "" {
			m.pending.toolCall.Input["command"] = edited
		}
		return m.approvePendingBash()
	case tea.KeyEsc:
		m.input.SetValue("")
		m.state = stateApproval
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

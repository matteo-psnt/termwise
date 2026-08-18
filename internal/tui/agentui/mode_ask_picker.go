package agentui

import tea "github.com/charmbracelet/bubbletea"

// askPickerMode is active when the agent's `ask` tool has surfaced a
// multiple-choice question and the user is choosing an answer.
type askPickerMode struct{}

func (askPickerMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pending.picker == nil {
		return m, nil
	}
	updated, result, submitted, cancelled := m.pending.picker.Update(msg)
	m.pending.picker = &updated

	if cancelled {
		result = "User cancelled"
		submitted = true
	}

	if submitted {
		m.appendThreadEntries(UserEntry{Content: result})
		m.refreshViewport()
		return m.resumePendingToolLoop(m.pending.result(result, false))
	}
	return m, nil
}

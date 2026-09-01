package agentui

import tea "charm.land/bubbletea/v2"

// askPickerMode is active when the agent's `ask` tool has surfaced a
// multiple-choice question and the user is choosing an answer.
type askPickerMode struct{}

func (askPickerMode) handleKey(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (askPickerMode) renderInputRow(m Model, _ int) string {
	if m.pending.picker == nil {
		return ""
	}
	return m.pending.picker.View(m.renderer)
}

// helpBindings preserves the pre-refactor behavior: picker screens fall through
// to the idle bindings. The picker's own controls are shown inline in its view.
func (askPickerMode) helpBindings(m Model) []binding {
	return idleMode{}.helpBindings(m)
}

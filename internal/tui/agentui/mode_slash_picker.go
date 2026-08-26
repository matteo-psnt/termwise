package agentui

import tea "github.com/charmbracelet/bubbletea"

// slashPickerMode is active while a slash-command sub-picker (e.g. /effort,
// /theme, /model) is open. Selection runs the corresponding command.
type slashPickerMode struct{}

func (slashPickerMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.slashPicker == nil {
		m.state = stateIdle
		return m, nil
	}
	updated, choice, submitted, cancelled := m.slashPicker.Update(msg)
	m.slashPicker = &updated
	if cancelled {
		m.slashPicker = nil
		m.state = stateIdle
		return m, nil
	}
	if submitted {
		cmd := findSlashCommand(updated.cmd)
		m.slashPicker = nil
		m.state = stateIdle
		if cmd == nil {
			return m, nil
		}
		return cmd.Run(m, []string{choice})
	}
	return m, nil
}

func (slashPickerMode) renderInputRow(m Model, _ int) string {
	if m.slashPicker == nil {
		return ""
	}
	return m.slashPicker.View(m.renderer)
}

// helpBindings preserves the pre-refactor behavior: picker screens fall through
// to the idle bindings. The picker's own controls are shown inline in its view.
func (slashPickerMode) helpBindings(m Model) []binding {
	return idleMode{}.helpBindings(m)
}

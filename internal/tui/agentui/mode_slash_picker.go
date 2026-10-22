package agentui

import tea "charm.land/bubbletea/v2"

// slashPickerMode is active while a slash-command sub-picker (e.g. /effort,
// /theme, /model) is open. Selection runs the corresponding command.
type slashPickerMode struct{}

func (slashPickerMode) handleKey(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.slashPicker == nil {
		m.state = stateIdle
		return m, nil
	}
	originalValue := m.slashPicker.current
	updated, action, choice := m.slashPicker.Update(msg)
	m.slashPicker = &updated
	cmd := findSlashCommand(updated.cmd)
	switch action {
	case pickerCancelled:
		m.slashPicker = nil
		m.state = stateIdle
		if cmd != nil && cmd.Preview != nil {
			m = cmd.Preview(m, originalValue)
			m.refreshViewport()
		}
	case pickerSubmitted:
		m.slashPicker = nil
		m.state = stateIdle
		if cmd != nil {
			// Undo the preview first so Run starts from the real state. Run only
			// commits on success; a failed /model commit would otherwise leave
			// the previewed providerName/modelID paired with the old client.
			if cmd.Preview != nil {
				m = cmd.Preview(m, originalValue)
			}
			return cmd.Run(m, []string{choice})
		}
	case pickerMoved:
		if cmd != nil && cmd.Preview != nil {
			m = cmd.Preview(m, updated.options[updated.cursor].Value)
			m.refreshViewport()
		}
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

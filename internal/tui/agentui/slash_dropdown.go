package agentui

import tea "charm.land/bubbletea/v2"

// openSlashPicker activates the slash-picker sub-TUI for the given command.
func (m Model) openSlashPicker(cmd, title, subtitle string, options []slashOption, current string) (tea.Model, tea.Cmd) {
	p := newSlashPicker(cmd, title, subtitle, options, current)
	m.slashPicker = &p
	m.state = stateSlashPicker
	return m, nil
}

// updateInputAndResetCompletions forwards the key to the textinput, and clears
// the transient state of both completion dropdowns (cursor + dismissed flag)
// whenever the input value changes.
func (m Model) updateInputAndResetCompletions(msg tea.Msg) (tea.Model, tea.Cmd) {
	prev := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prev {
		m.slashSel.reset()
		m.atSel.reset()
	}
	return m, cmd
}

// visibleSlashMatches returns the dropdown rows to render now, or nil if the
// dropdown should be hidden (no leading slash, no matches, or user-dismissed).
func (m Model) visibleSlashMatches() []slashMatch {
	if m.slashSel.closed || m.histIdx >= 0 {
		return nil
	}
	matches, _ := computeSlashMatches(m, m.input.Value())
	return matches
}

// handleSlashDropdownKey handles navigation and acceptance keys when the
// dropdown is visible. Returns (newModel, handled). When handled is false,
// the caller should fall through to the normal idle-key handler.
func (m Model) handleSlashDropdownKey(msg tea.KeyPressMsg, matches []slashMatch) (Model, bool) {
	if msg.Code == tea.KeyEnter {
		// Enter accepts the highlighted match and submits in one step, so a
		// dropdown selection always executes the right command (not whatever
		// raw prefix the user typed).
		m.slashSel.clamp(len(matches))
		return m.acceptCompletion(matches, &m.slashSel), false
	}
	return m.handleCompletionNav(msg, matches, &m.slashSel)
}

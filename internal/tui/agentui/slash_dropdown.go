package agentui

import tea "charm.land/bubbletea/v2"

// openSlashPicker activates the slash-picker sub-TUI for the given command.
func (m Model) openSlashPicker(cmd, title, subtitle string, options []slashOption, current string) (tea.Model, tea.Cmd) {
	p := newSlashPicker(cmd, title, subtitle, options, current)
	m.slashPicker = &p
	m.state = stateSlashPicker
	return m, nil
}

// updateInputAndResetSlash forwards the key to the textinput, and clears the
// transient slash dropdown state (cursor + dismissed flag) whenever the input
// value changes.
func (m Model) updateInputAndResetSlash(msg tea.Msg) (tea.Model, tea.Cmd) {
	prev := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prev {
		m.slashCursor = 0
		m.slashClosed = false
		m.atCursor = 0
		m.atClosed = false
	}
	return m, cmd
}

// visibleSlashMatches returns the dropdown rows to render now, or nil if the
// dropdown should be hidden (no leading slash, no matches, or user-dismissed).
func (m Model) visibleSlashMatches() []slashMatch {
	if m.slashClosed || m.histIdx >= 0 {
		return nil
	}
	matches, _ := computeSlashMatches(m, m.input.Value())
	return matches
}

// handleSlashDropdownKey handles navigation and acceptance keys when the
// dropdown is visible. Returns (newModel, handled). When handled is false,
// the caller should fall through to the normal idle-key handler.
func (m Model) handleSlashDropdownKey(msg tea.KeyPressMsg, matches []slashMatch) (Model, bool) {
	if m.slashCursor >= len(matches) {
		m.slashCursor = len(matches) - 1
	}
	if m.slashCursor < 0 {
		m.slashCursor = 0
	}
	switch msg.Code {
	case tea.KeyUp:
		if m.slashCursor > 0 {
			m.slashCursor--
		}
		return m, true
	case tea.KeyDown:
		if m.slashCursor < len(matches)-1 {
			m.slashCursor++
		}
		return m, true
	case tea.KeyTab:
		return m.acceptSlashCompletion(matches), true
	case tea.KeyEnter:
		// Enter accepts the highlighted match and submits in one step, so a
		// dropdown selection always executes the right command (not whatever
		// raw prefix the user typed).
		m = m.acceptSlashCompletion(matches)
		return m, false
	case tea.KeyEscape:
		m.slashClosed = true
		return m, true
	}
	switch msg.String() {
	case "ctrl+p":
		if m.slashCursor > 0 {
			m.slashCursor--
		}
		return m, true
	case "ctrl+n":
		if m.slashCursor < len(matches)-1 {
			m.slashCursor++
		}
		return m, true
	case "right":
		// Only consume Right at end-of-line so cursor movement still works mid-text.
		if m.inputCursorAtEnd() {
			return m.acceptSlashCompletion(matches), true
		}
	}
	return m, false
}

// acceptSlashCompletion appends the highlighted match's completion suffix to
// the input and resets the dropdown cursor.
func (m Model) acceptSlashCompletion(matches []slashMatch) Model {
	if m.slashCursor < 0 || m.slashCursor >= len(matches) {
		return m
	}
	completion := matches[m.slashCursor].Completion
	if completion == "" {
		return m
	}
	m.input.SetValue(m.input.Value() + completion)
	m.input.CursorEnd()
	m.slashCursor = 0
	return m
}

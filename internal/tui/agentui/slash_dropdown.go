package agentui

import tea "charm.land/bubbletea/v2"

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

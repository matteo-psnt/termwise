package agentui

import tea "charm.land/bubbletea/v2"

// visibleSlashMatches returns the dropdown rows to render now, or nil if the
// dropdown should be hidden (no leading slash, no matches, or user-dismissed).
func (m Model) visibleSlashMatches() []slashMatch {
	if m.slashSel.closed || m.histIdx >= 0 {
		return nil
	}
	value := m.input.Value()
	if c := m.slashCache; c != nil && c.valid && c.value == value && c.providerName == m.providerName {
		return c.matches
	}
	matches, _ := computeSlashMatches(m, value)
	if m.slashCache != nil {
		*m.slashCache = slashMatchCache{valid: true, value: value, providerName: m.providerName, matches: matches}
	}
	return matches
}

// slashMatchCache holds the last computeSlashMatches result. /model argument
// completion reads the config file, and visibleSlashMatches runs several times
// per keystroke and on every frame (key handling, the input row, the
// dropdown, the layout height); caching per input value turns that into one
// read per change of input. providerName is part of the key because /model
// sorts the active provider first.
type slashMatchCache struct {
	valid        bool
	value        string
	providerName string
	matches      []slashMatch
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

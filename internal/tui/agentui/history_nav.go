package agentui

import "strings"

// cancelHistoryNav exits prompt-history browsing, discarding the saved draft.
func (m *Model) cancelHistoryNav() {
	m.histIdx = -1
	m.histDraft = ""
}

// historyBack moves one step older in prompt history.
func (m Model) historyBack() Model {
	if m.promptHistory == nil {
		return m
	}
	entries := m.promptHistory.Entries()
	if len(entries) == 0 {
		return m
	}
	if m.histIdx < 0 {
		// Save the current draft before navigating.
		m.histDraft = m.input.Value()
	}
	next := m.histIdx + 1
	if next >= len(entries) {
		return m
	}
	m.histIdx = next
	m.input.SetValue(entries[m.histIdx])
	m.input.CursorEnd()
	return m
}

// historyForward moves one step newer in prompt history, restoring the draft
// when moving past the most recent entry.
func (m Model) historyForward() Model {
	if m.histIdx < 0 {
		return m
	}
	m.histIdx--
	if m.histIdx < 0 {
		m.input.SetValue(m.histDraft)
		m.histDraft = ""
	} else if m.promptHistory != nil {
		entries := m.promptHistory.Entries()
		if m.histIdx < len(entries) {
			m.input.SetValue(entries[m.histIdx])
		}
	}
	m.input.CursorEnd()
	return m
}

// enterHistSearch switches to stateHistSearch and seeds the initial match list.
func (m Model) enterHistSearch() Model {
	m.histDraft = m.input.Value()
	m.histSearch = histSearchState{}
	m.state = stateHistSearch
	// Seed matches from current input value as query.
	m.histSearch.query = m.histDraft
	m.histSearch.matches = m.filterHistory(m.histSearch.query)
	m.histSearch.idx = 0
	if len(m.histSearch.matches) > 0 {
		m.input.SetValue(m.histSearch.matches[0])
		m.input.CursorEnd()
	}
	return m
}

// filterHistory returns history entries that contain query as a substring,
// in newest-first order.
func (m Model) filterHistory(query string) []string {
	if m.promptHistory == nil {
		return nil
	}
	entries := m.promptHistory.Entries()
	if query == "" {
		return entries
	}
	var out []string
	q := strings.ToLower(query)
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e), q) {
			out = append(out, e)
		}
	}
	return out
}

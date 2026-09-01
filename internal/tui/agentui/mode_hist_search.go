package agentui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// histSearchMode is active during Ctrl+R reverse search through the prompt
// history. The mode reads m.histSearch off Model; it mutates that state in
// place as the user types, navigates matches, or commits / aborts the search.
type histSearchMode struct{}

var histSearchKeys = keymap{
	{keys: []string{"enter"}, run: Model.histSearchAccept},
	{keys: []string{"esc"}, run: Model.histSearchCancel},
	{keys: []string{"up"}, run: Model.histSearchOlder},
	{keys: []string{"down"}, run: Model.histSearchNewer},
	{keys: []string{"ctrl+r"}, run: Model.histSearchCycle},
}

func (histSearchMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if newM, cmd, ok := histSearchKeys.handle(m, msg); ok {
		return newM, cmd
	}

	switch msg.Type {
	case tea.KeyBackspace, tea.KeyDelete:
		q := m.histSearch.query
		if len(q) > 0 {
			q = q[:len(q)-1]
		}
		m.setHistQuery(q)
	default:
		// Printable character: extend the search query.
		if len(msg.Runes) > 0 {
			m.setHistQuery(m.histSearch.query + string(msg.Runes))
		}
	}
	return m, nil
}

// histSearchAccept keeps the current match and returns to idle.
func (m Model) histSearchAccept() (tea.Model, tea.Cmd) {
	m.state = stateIdle
	m.histSearch = histSearchState{}
	m.input.CursorEnd()
	return m, nil
}

// histSearchCancel aborts the search and restores the original draft.
func (m Model) histSearchCancel() (tea.Model, tea.Cmd) {
	m.state = stateIdle
	m.input.SetValue(m.histDraft)
	m.histSearch = histSearchState{}
	m.histDraft = ""
	m.input.CursorEnd()
	return m, nil
}

// histSearchOlder moves to an older match (matches are newest-first).
func (m Model) histSearchOlder() (tea.Model, tea.Cmd) {
	if len(m.histSearch.matches) == 0 {
		return m, nil
	}
	m.histSearch.idx = min(m.histSearch.idx+1, len(m.histSearch.matches)-1)
	m.showHistMatch()
	return m, nil
}

// histSearchNewer moves to a newer match.
func (m Model) histSearchNewer() (tea.Model, tea.Cmd) {
	if len(m.histSearch.matches) == 0 {
		return m, nil
	}
	m.histSearch.idx = max(m.histSearch.idx-1, 0)
	m.showHistMatch()
	return m, nil
}

// histSearchCycle advances to the next match, wrapping around (Ctrl+R again).
func (m Model) histSearchCycle() (tea.Model, tea.Cmd) {
	if len(m.histSearch.matches) > 0 {
		m.histSearch.idx = (m.histSearch.idx + 1) % len(m.histSearch.matches)
		m.showHistMatch()
	}
	return m, nil
}

// showHistMatch sets the input to the currently selected match.
func (m *Model) showHistMatch() {
	m.input.SetValue(m.histSearch.matches[m.histSearch.idx])
	m.input.CursorEnd()
}

// setHistQuery re-filters history for query and shows the top match (or the raw
// query when nothing matches). Shared by the backspace and typing paths.
func (m *Model) setHistQuery(query string) {
	m.histSearch.query = query
	m.histSearch.matches = m.filterHistory(query)
	m.histSearch.idx = 0
	if len(m.histSearch.matches) > 0 {
		m.input.SetValue(m.histSearch.matches[0])
	} else {
		m.input.SetValue(query)
	}
	m.input.CursorEnd()
}

func (histSearchMode) renderInputRow(m Model, _ int) string {
	query := m.histSearch.query
	count := len(m.histSearch.matches)
	hint := fmt.Sprintf(" (%d)", count)
	return m.renderer.styles.InputPrompt.Render(" / "+query+hint+" › ") + m.input.View()
}

// helpBindings preserves the pre-refactor behavior: hist-search falls through
// to the idle bindings. The search controls (↑↓ / ctrl+r / esc / ↵) are
// implicit to the reverse-search convention.
func (histSearchMode) helpBindings(m Model) []binding {
	return idleMode{}.helpBindings(m)
}

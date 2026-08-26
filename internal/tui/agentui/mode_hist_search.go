package agentui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// histSearchMode is active during Ctrl+R reverse search through the prompt
// history. The mode reads m.histSearch off Model; it mutates that state in
// place as the user types, navigates matches, or commits / aborts the search.
type histSearchMode struct{}

func (histSearchMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		// Accept the current match and return to idle.
		m.state = stateIdle
		m.histSearch = histSearchState{}
		m.input.CursorEnd()
		return m, nil

	case tea.KeyEsc:
		// Cancel search: restore original draft.
		m.state = stateIdle
		m.input.SetValue(m.histDraft)
		m.histSearch = histSearchState{}
		m.histDraft = ""
		m.input.CursorEnd()
		return m, nil

	case tea.KeyUp, tea.KeyDown:
		// Cycle through matches.
		if len(m.histSearch.matches) == 0 {
			return m, nil
		}
		if msg.Type == tea.KeyUp {
			m.histSearch.idx++
			if m.histSearch.idx >= len(m.histSearch.matches) {
				m.histSearch.idx = len(m.histSearch.matches) - 1
			}
		} else {
			m.histSearch.idx--
			if m.histSearch.idx < 0 {
				m.histSearch.idx = 0
			}
		}
		m.input.SetValue(m.histSearch.matches[m.histSearch.idx])
		m.input.CursorEnd()
		return m, nil

	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.histSearch.query) > 0 {
			m.histSearch.query = m.histSearch.query[:len(m.histSearch.query)-1]
		}
		m.histSearch.matches = m.filterHistory(m.histSearch.query)
		m.histSearch.idx = 0
		if len(m.histSearch.matches) > 0 {
			m.input.SetValue(m.histSearch.matches[0])
		} else {
			m.input.SetValue(m.histSearch.query)
		}
		m.input.CursorEnd()
		return m, nil

	default:
		if msg.String() == "ctrl+r" {
			// Ctrl+R again: cycle to next match.
			if len(m.histSearch.matches) > 0 {
				m.histSearch.idx = (m.histSearch.idx + 1) % len(m.histSearch.matches)
				m.input.SetValue(m.histSearch.matches[m.histSearch.idx])
				m.input.CursorEnd()
			}
			return m, nil
		}
		// Printable character: extend the search query.
		if len(msg.Runes) > 0 {
			m.histSearch.query += string(msg.Runes)
			m.histSearch.matches = m.filterHistory(m.histSearch.query)
			m.histSearch.idx = 0
			if len(m.histSearch.matches) > 0 {
				m.input.SetValue(m.histSearch.matches[0])
			} else {
				m.input.SetValue(m.histSearch.query)
			}
			m.input.CursorEnd()
		}
		return m, nil
	}
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

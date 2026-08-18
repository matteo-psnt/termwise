package agentui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// idleMode is the default mode: no modal screen is open, the user can type a
// new prompt, navigate prompt history, scroll the viewport, or open the
// always-on slash dropdown by leading with "/".
type idleMode struct{}

func (idleMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Slash-command dropdown overrides come first so they shadow history nav,
	// the prompt-suggestion tab handler, and Esc-stash gestures. When the
	// override doesn't fully handle the key (e.g. Enter accepts the highlight
	// and falls through to submit), we still adopt its model mutations.
	if matches := m.visibleSlashMatches(); len(matches) > 0 {
		newM, handled := m.handleSlashDropdownKey(msg, matches)
		if handled {
			return newM, nil
		}
		m = newM
	}

	switch msg.Type {
	case tea.KeyEnter:
		return m.submitCurrentInput()

	case tea.KeyTab:
		if s := m.visibleSuggestion(); s != "" {
			m.input.SetValue(s)
			m.input.CursorEnd()
			m.suggestion = ""
			return m, nil
		}
		return m.updateInputAndResetSlash(msg)

	case tea.KeyUp:
		return m.historyBack(), nil

	case tea.KeyDown:
		return m.historyForward(), nil

	case tea.KeyPgUp:
		m.vp.PageUp()
		m.userScrolled = !m.vp.AtBottom()
		return m, nil

	case tea.KeyPgDown:
		m.vp.PageDown()
		m.userScrolled = !m.vp.AtBottom()
		return m, nil

	case tea.KeyEnd:
		m.userScrolled = false
		m.vp.GotoBottom()
		return m, nil

	case tea.KeyEsc:
		if m.input.Value() == "" {
			return m, nil
		}
		if !m.lastEscAt.IsZero() && time.Since(m.lastEscAt) < 500*time.Millisecond {
			text := m.input.Value()
			m.input.SetValue("")
			m.input.CursorEnd()
			m.lastEscAt = time.Time{}
			if m.promptHistory != nil {
				_ = m.promptHistory.Push(text)
			}
			return m, nil
		}
		m.lastEscAt = time.Now()
		return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
			return escTimeoutMsg{}
		})

	default:
		if msg.String() == "ctrl+r" {
			return m.enterHistSearch(), nil
		}
		// Any other typing cancels history navigation.
		if m.histIdx >= 0 {
			m.histIdx = -1
			m.histDraft = ""
		}
		return m.updateInputAndResetSlash(msg)
	}
}

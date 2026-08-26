package agentui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func (idleMode) renderInputRow(m Model, _ int) string {
	input := m.input
	input.Placeholder = ""
	prompt := m.renderer.styles.InputPrompt.Render(" › ")

	// Slash-command dropdown and inline ghost take precedence over prompt
	// suggestions whenever the dropdown is visible.
	matches := m.visibleSlashMatches()
	var suggestion string
	if len(matches) > 0 && m.slashCursor < len(matches) {
		suggestion = m.renderer.styles.Suggestion.Render(matches[m.slashCursor].Completion)
	} else if s := m.visibleSuggestion(); s != "" {
		suggestion = m.renderer.styles.Suggestion.Render(s)
	}

	line := prompt + input.View() + suggestion
	if len(matches) > 0 {
		return m.renderSlashDropdown(matches) + "\n" + line
	}
	return line
}

func (idleMode) helpBindings(_ Model) []binding {
	return []binding{
		{"↵", "submit"},
		{"tab", "accept suggestion"},
		{"↑ / ↓", "history"},
		{"ctrl+r", "search history"},
		{"/", "slash commands (/help)"},
		{"?", "close help"},
		{"ctrl+c", "quit"},
	}
}

// renderSlashDropdown renders the slash-command match list above the input
// row. Highlights the cursor row and scrolls a windowed view when matches
// exceed slashDropdownMaxRows.
const slashDropdownMaxRows = 5

func (m Model) renderSlashDropdown(matches []slashMatch) string {
	start := 0
	end := len(matches)
	if end > slashDropdownMaxRows {
		if m.slashCursor >= slashDropdownMaxRows {
			start = m.slashCursor - slashDropdownMaxRows + 1
		}
		end = start + slashDropdownMaxRows
		if end > len(matches) {
			end = len(matches)
			start = end - slashDropdownMaxRows
		}
	}
	visible := matches[start:end]

	labelW := 0
	for _, mt := range visible {
		if w := lipgloss.Width(mt.Label); w > labelW {
			labelW = w
		}
	}

	var b strings.Builder
	for i, mt := range visible {
		marker := "  "
		var labelOut string
		if start+i == m.slashCursor {
			marker = m.renderer.styles.InputPrompt.Render("› ")
			labelOut = m.renderer.styles.InputPrompt.Bold(true).Render(mt.Label)
		} else {
			labelOut = mt.Label
		}
		b.WriteString(marker + labelOut)
		if mt.Description != "" {
			pad := max(labelW-lipgloss.Width(mt.Label)+2, 1)
			b.WriteString(strings.Repeat(" ", pad) + m.renderer.styles.ActionHints.Render(mt.Description))
		}
		if i < len(visible)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

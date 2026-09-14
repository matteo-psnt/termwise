package agentui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// idleMode is the default mode: no modal screen is open, the user can type a
// new prompt, navigate prompt history, scroll the viewport, or open the
// always-on slash dropdown by leading with "/".
type idleMode struct{}

// idleKeys are the discrete command keys for idle. They carry no label: the
// idle help overlay is hand-written (it lists informational rows like "/" that
// aren't bindings), so these never render. Tab and plain typing are not in the
// table because they need the raw msg; they're handled as residual cases.
// maxInputRows caps how tall the prompt input grows before it scrolls
// internally. Multi-line input is entered with Shift+Enter, Alt+Enter, or a
// trailing backslash before Enter.
const maxInputRows = 10

var idleKeys = append(keymap{
	{keys: []string{"enter"}, run: Model.submitCurrentInput},
	{keys: []string{"shift+enter", "alt+enter"}, run: Model.insertInputNewline},
	{keys: []string{"up"}, run: Model.idleUp},
	{keys: []string{"down"}, run: Model.idleDown},
	{keys: []string{"esc"}, run: Model.idleEsc},
	{keys: []string{"ctrl+r"}, run: func(m Model) (tea.Model, tea.Cmd) { return m.enterHistSearch(), nil }},
}, scrollKeys...)

// insertInputNewline inserts a literal newline at the cursor. Bound to
// Shift+Enter / Alt+Enter (a real Shift+Enter only reaches us on terminals
// that speak the Kitty keyboard protocol; the backslash-Enter path in
// submitCurrentInput is the portable fallback).
func (m Model) insertInputNewline() (tea.Model, tea.Cmd) {
	m.cancelHistoryNav()
	m.input.InsertString("\n")
	return m, nil
}

// idleUp moves the cursor up one line inside the input, falling through to
// prompt-history navigation only when already on the first line.
func (m Model) idleUp() (tea.Model, tea.Cmd) {
	if m.input.Line() > 0 {
		m.input.CursorUp()
		return m, nil
	}
	return m.historyBack(), nil
}

// idleDown mirrors idleUp: move down a line, else step forward through history.
func (m Model) idleDown() (tea.Model, tea.Cmd) {
	if m.input.Line() < strings.Count(m.input.Value(), "\n") {
		m.input.CursorDown()
		return m, nil
	}
	return m.historyForward(), nil
}

// inputCursorAtEnd reports whether the cursor sits at the very end of the input
// text (last line, end of that line). Used by the slash dropdown to decide
// whether Right accepts a completion or just moves the cursor.
func (m Model) inputCursorAtEnd() bool {
	onLastLine := m.input.Line() == strings.Count(m.input.Value(), "\n")
	li := m.input.LineInfo()
	return onLastLine && li.CharOffset >= li.CharWidth
}

func (idleMode) handleKey(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

	// The @-dropdown is checked before the Tab handler below: with a path
	// fragment typed there is often a pending ghost suggestion too, and Tab
	// must complete the path the user is looking at rather than accept it.
	if matches := m.visibleAtMatches(); len(matches) > 0 {
		newM, handled := m.handleAtDropdownKey(msg, matches)
		if handled {
			return newM, nil
		}
		m = newM
	}

	// Tab needs the raw msg (accept the ghost suggestion, or insert a tab), so
	// it's handled before the table rather than as a binding.
	if msg.Code == tea.KeyTab {
		if s := m.visibleSuggestion(); s != "" {
			m.input.SetValue(s)
			m.input.CursorEnd()
			m.suggestion = ""
			return m, nil
		}
		return m.updateInputAndResetSlash(msg)
	}

	if newM, cmd, ok := idleKeys.handle(m, msg); ok {
		return newM, cmd
	}

	// Any other typing cancels history navigation, then forwards to the input.
	if m.histIdx >= 0 {
		m.cancelHistoryNav()
	}
	return m.updateInputAndResetSlash(msg)
}

// idleEsc implements the two-tap Esc gesture: the first Esc arms a 500ms timer
// (shown in the status line), a second Esc within the window stashes the draft
// to history and clears the input.
func (m Model) idleEsc() (tea.Model, tea.Cmd) {
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
}

func (idleMode) renderInputRow(m Model, _ int) string {
	input := m.input
	input.Placeholder = ""
	prompt := m.renderer.styles.InputPrompt.Render(" › ")

	// Inline ghost suggestion is hidden while the dropdown is visible — the
	// dropdown already shows the highlighted completion, and rendering both
	// makes the input line overflow m.width and wrap.
	matches := m.visibleSlashMatches()
	var suggestion string
	if len(matches) == 0 && len(m.visibleAtMatches()) == 0 {
		if s := m.visibleSuggestion(); s != "" {
			suggestion = m.renderer.styles.Suggestion.Render(s)
		}
	}

	// textarea pads every line to its full width with (now-unstyled) spaces;
	// trim that so the prompt sits flush and the ghost suggestion doesn't get
	// pushed past the edge and wrap. The prompt prefixes the first line;
	// continuation lines align under the text.
	indent := strings.Repeat(" ", lipgloss.Width(prompt))
	lines := strings.Split(input.View(), "\n")
	for i, ln := range lines {
		ln = strings.TrimRight(ln, " ")
		if i == 0 {
			lines[i] = prompt + ln
		} else {
			lines[i] = indent + ln
		}
	}
	return strings.Join(lines, "\n") + suggestion
}

// helpBindings is hand-written rather than derived from idleKeys: it lists
// informational rows (Tab, "/") that aren't simple key→action bindings, and
// merges ↑/↓ into one row. The global ?/ctrl+c rows are appended centrally by
// renderHelpOverlay.
func (idleMode) helpBindings(_ Model) []binding {
	return []binding{
		{"↵", "submit"},
		{"shift+↵", "newline (or \\↵)"},
		{"tab", "accept suggestion"},
		{"↑ / ↓", "history"},
		{"ctrl+r", "search history"},
		{"/", "slash commands (/help)"},
		{"@", "complete a file path"},
	}
}

// renderSlashDropdown renders the slash-command match list below the input
// row. The selected row uses the accent color; the other rows are faint to
// keep the dropdown unobtrusive. Scrolls a windowed view when matches exceed
// slashDropdownMaxRows.
const slashDropdownMaxRows = 5

func (m Model) renderSlashDropdown(matches []slashMatch, cursor int) string {
	start := 0
	end := len(matches)
	if end > slashDropdownMaxRows {
		if cursor >= slashDropdownMaxRows {
			start = cursor - slashDropdownMaxRows + 1
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
		var labelOut string
		if start+i == cursor {
			labelOut = m.renderer.styles.InputPrompt.Render(mt.Label)
		} else {
			labelOut = m.renderer.styles.ActionHints.Render(mt.Label)
		}
		b.WriteString(labelOut)
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

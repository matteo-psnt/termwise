package agentui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The prompt has two completion dropdowns — slash commands and @ paths. They
// differ in what they list and in what Enter means, but the highlight moves,
// gets dismissed, and gets accepted identically in both. That shared half lives
// here; each dropdown keeps only its own trigger, its rows, and its Enter.

// completionState is a dropdown's transient selection: which row is
// highlighted, and whether the user has dismissed it with Esc. Both are reset
// whenever the input value changes.
type completionState struct {
	cursor int
	closed bool
}

// reset returns the state to a freshly-opened dropdown.
func (c *completionState) reset() {
	c.cursor = 0
	c.closed = false
}

// clamp keeps the cursor inside a match list of n rows. Matches are recomputed
// from the input on every keystroke, so a list that shrank under the cursor is
// the normal case, not an error.
func (c *completionState) clamp(n int) {
	if c.cursor >= n {
		c.cursor = n - 1
	}
	if c.cursor < 0 {
		c.cursor = 0
	}
}

// navigate applies the keys that only move the highlight, reporting whether the
// key was one of them.
func (c *completionState) navigate(msg tea.KeyPressMsg, n int) bool {
	switch {
	case msg.Code == tea.KeyUp, msg.String() == "ctrl+p":
		if c.cursor > 0 {
			c.cursor--
		}
		return true
	case msg.Code == tea.KeyDown, msg.String() == "ctrl+n":
		if c.cursor < n-1 {
			c.cursor++
		}
		return true
	}
	return false
}

// handleCompletionNav applies the keys both dropdowns treat the same way:
// moving the highlight, dismissing with Esc, and accepting with Tab or with
// Right at end-of-line. Returns handled=false for anything else, so ordinary
// typing reaches the input.
//
// Callers deal with Enter before delegating here — it is the one key that means
// different things in the two dropdowns — so this never sees it.
func (m Model) handleCompletionNav(msg tea.KeyPressMsg, matches []slashMatch, sel *completionState) (Model, bool) {
	sel.clamp(len(matches))
	if sel.navigate(msg, len(matches)) {
		return m, true
	}
	switch msg.Code {
	case tea.KeyTab:
		return m.acceptCompletion(matches, sel), true
	case tea.KeyEscape:
		sel.closed = true
		return m, true
	}
	// Only consume Right at end-of-line so cursor movement still works mid-text.
	if msg.String() == "right" && m.inputCursorAtEnd() {
		return m.acceptCompletion(matches, sel), true
	}
	return m, false
}

// acceptCompletion appends the highlighted row's completion suffix to the input
// and returns the cursor to the top.
func (m Model) acceptCompletion(matches []slashMatch, sel *completionState) Model {
	if sel.cursor < 0 || sel.cursor >= len(matches) {
		return m
	}
	match := matches[sel.cursor]
	if match.Completion == "" {
		return m
	}
	value := m.input.Value()
	m.input.SetValue(value[:len(value)-min(match.Replace, len(value))] + match.Completion)
	m.input.CursorEnd()
	sel.cursor = 0
	return m
}

// visibleDropdown returns the rows and highlight index for whichever completion
// dropdown is open, so the layout and render sites do not each have to restate
// the precedence between them. cursor is meaningless when matches is empty.
func (m Model) visibleDropdown() (matches []slashMatch, cursor int) {
	if slash := m.visibleSlashMatches(); len(slash) > 0 {
		return slash, m.slashSel.cursor
	}
	return m.visibleAtMatches(), m.atSel.cursor
}

// dropdownMaxRows is how many rows a completion dropdown shows at once; longer
// match lists scroll.
const dropdownMaxRows = 5

// renderCompletionDropdown renders a completion dropdown's match list (slash
// commands or @ paths) below the input row. The selected row uses the accent
// color; the other rows are faint to keep the dropdown unobtrusive. Scrolls a
// windowed view when matches exceed dropdownMaxRows.
func (m Model) renderCompletionDropdown(matches []slashMatch, cursor int) string {
	start := 0
	end := len(matches)
	if end > dropdownMaxRows {
		if cursor >= dropdownMaxRows {
			start = cursor - dropdownMaxRows + 1
		}
		end = start + dropdownMaxRows
		if end > len(matches) {
			end = len(matches)
			start = end - dropdownMaxRows
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

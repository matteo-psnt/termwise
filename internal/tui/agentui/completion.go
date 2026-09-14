package agentui

import tea "charm.land/bubbletea/v2"

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
// Right at end-of-line. Returns handled=false for any key the caller has to
// decide on itself — in practice Enter, which is the one key whose meaning
// differs between the two.
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
	completion := matches[sel.cursor].Completion
	if completion == "" {
		return m
	}
	m.input.SetValue(m.input.Value() + completion)
	m.input.CursorEnd()
	sel.cursor = 0
	return m
}

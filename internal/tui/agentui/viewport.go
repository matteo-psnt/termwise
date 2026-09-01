package agentui

import "strings"

// popupHeight is the maximum number of terminal lines the TUI occupies.
// Inline mode (no alt screen) renders in place, so we cap the height to
// keep it feeling like a sized popup rather than a full-page takeover.
const popupHeight = 30

// inputRowHeight returns the row count of the input section between the
// viewport's bottom separator and the status line's top separator.
func (m Model) inputRowHeight() int {
	switch m.state {
	case stateApproval, stateCommandProposal:
		return 4 // 3-line command block + 1 action-hints line
	case stateAskPicker:
		if m.pending.picker != nil {
			return strings.Count(m.pending.picker.View(m.renderer), "\n") + 1
		}
	case stateSlashPicker:
		if m.slashPicker != nil {
			return strings.Count(m.slashPicker.View(m.renderer), "\n") + 1
		}
	case stateConfigEditor:
		if m.configEditor != nil {
			return strings.Count(configEditorMode{}.renderInputRow(m, 0), "\n") + 1
		}
	case stateIdle:
		// The idle prompt is a textarea that grows with multi-line input.
		return strings.Count(idleMode{}.renderInputRow(m, 0), "\n") + 1
	}
	return 1
}

// totalViewHeight returns the total row count of the View() output for the
// current state: header(1) + viewport + sep(1) + input + sep(1) + bottom.
// The bottom region is the status line normally, or the slash dropdown when
// it's visible.
func (m Model) totalViewHeight() int {
	_, vpH := m.viewportDims()
	bottomH := 1 // status line
	if m.state == stateIdle {
		if matches := m.visibleSlashMatches(); len(matches) > 0 {
			bottomH = min(len(matches), slashDropdownMaxRows)
		}
	}
	return 3 + vpH + m.inputRowHeight() + bottomH
}

// viewportDims calculates viewport dimensions from terminal size.
func (m Model) viewportDims() (width, height int) {
	// Layout: header(1) | viewport | sep(1) | input(1) | sep(1) | status(1)
	innerW := max(m.width, 10)
	h := popupHeight
	if m.height > 0 && m.height < h {
		h = m.height
	}
	innerH := max(h-5, 3) // header(1) + sep(1) + input(1) + sep(1) + status(1)
	return innerW, innerH
}

// refreshViewport re-renders the thread, updates viewport content, and
// re-indexes URL positions for click-to-open handling.
func (m *Model) refreshViewport() {
	content := m.renderer.RenderThread(m.thread)
	if content == "" {
		content = m.emptyStateHint()
	}
	m.vp.SetContent(content)
	m.links = findLinks(content)
	if !m.userScrolled {
		m.vp.GotoBottom()
	}
}

// emptyStateHint returns the placeholder text shown when the thread is empty.
func (m Model) emptyStateHint() string {
	dim := m.renderer.styles.ActionHints
	key := m.renderer.styles.HelpKey
	return dim.Render("  Ready. Ask anything, or type ") +
		key.Render("/") +
		dim.Render(" for commands.")
}

package agentui

import (
	"fmt"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// copyToastTimeoutMsg fires after the copy confirmation should disappear.
type copyToastTimeoutMsg struct{}

// handleMouseMsg owns the always-on mouse-driven selection + link-click
// widget. Returns (handled, model, cmd) — handled=false lets the caller fall
// through to wheel scrolling and the active mode's keyboard logic.
//
// v2 splits mouse events into distinct message types (click/motion/release/
// wheel) rather than a single message with an Action field.
func (m Model) handleMouseMsg(msg tea.MouseMsg) (bool, tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	switch msg.(type) {
	case tea.MouseClickMsg:
		if mouse.Button != tea.MouseLeft {
			return false, m, nil
		}
		return m.handleMousePress(mouse)
	case tea.MouseMotionMsg:
		if !m.selection.active {
			return false, m, nil
		}
		return m.handleMouseMotion(mouse)
	case tea.MouseReleaseMsg:
		if !m.selection.active {
			return false, m, nil
		}
		newM, cmd := m.finishSelection()
		return true, newM, cmd
	}
	return false, m, nil
}

// handleMousePress begins (or, for terminals that never emit release, ends and
// restarts) a selection, or follows a clicked link.
func (m Model) handleMousePress(mouse tea.Mouse) (bool, tea.Model, tea.Cmd) {
	totalH := m.totalViewHeight()
	line := mouse.Y - m.tuiTopRow
	col := mouse.X
	inBlock := line >= 0 && line < totalH

	// Some terminals (zsh ZLE widget host) never emit release and encode
	// drag-end as another press. Treat a press during an active drag as
	// that missed release.
	var copyCmd tea.Cmd
	if m.selection.active {
		m, copyCmd = m.finishSelection()
	}
	m.selection = selectionState{}
	if !inBlock {
		return copyCmd != nil, m, copyCmd
	}
	_, vpH := m.viewportDims()
	if line >= 1 && line < 1+vpH {
		vpLine := line - 1 + m.vp.YOffset()
		for _, l := range m.links {
			if l.Line == vpLine && col >= l.StartX && col < l.EndX {
				_ = openURL(l.URL)
				return true, m, copyCmd
			}
		}
	}
	m.selection = selectionState{
		active:    true,
		startLine: line, startCol: col,
		endLine: line, endCol: col,
	}
	return true, m, copyCmd
}

// handleMouseMotion extends the active selection to the dragged-to cell.
func (m Model) handleMouseMotion(mouse tea.Mouse) (bool, tea.Model, tea.Cmd) {
	totalH := m.totalViewHeight()
	line := mouse.Y - m.tuiTopRow
	if line < 0 {
		line = 0
	} else if line >= totalH {
		line = totalH - 1
	}
	m.selection.endLine = line
	m.selection.endCol = mouse.X
	return true, m, nil
}

// finishSelection copies the current selection to the clipboard and starts
// the toast-timeout tick.
func (m Model) finishSelection() (Model, tea.Cmd) {
	m.selection.active = false
	if !m.selection.has() {
		m.selection = selectionState{}
		return m, nil
	}
	text := extractSelection(m.renderedView(), m.selection)
	if text == "" {
		m.selection = selectionState{}
		return m, nil
	}
	_ = copyToClipboard(text)
	n := utf8.RuneCountInString(text)
	unit := "chars"
	if n == 1 {
		unit = "char"
	}
	m.copyToastMsg = fmt.Sprintf("Copied %d %s to clipboard", n, unit)
	m.copyToastUntil = time.Now().Add(2500 * time.Millisecond)
	return m, tea.Tick(2500*time.Millisecond, func(time.Time) tea.Msg {
		return copyToastTimeoutMsg{}
	})
}

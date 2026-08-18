package agentui

import (
	"fmt"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// copyToastTimeoutMsg fires after the copy confirmation should disappear.
type copyToastTimeoutMsg struct{}

// handleMouseMsg owns the always-on mouse-driven selection + link-click
// widget. Returns (handled, model, cmd) — handled=false lets the caller fall
// through to the active mode's keyboard logic.
func (m Model) handleMouseMsg(msg tea.MouseMsg) (bool, tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionPress && msg.Button != tea.MouseButtonLeft {
		return false, m, nil
	}
	if msg.Action != tea.MouseActionPress && !m.selection.active {
		return false, m, nil
	}

	totalH := m.totalViewHeight()
	line := msg.Y - m.tuiTopRow
	col := msg.X
	inBlock := line >= 0 && line < totalH

	switch msg.Action {
	case tea.MouseActionPress:
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
			vpLine := line - 1 + m.vp.YOffset
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

	case tea.MouseActionMotion:
		if line < 0 {
			line = 0
		} else if line >= totalH {
			line = totalH - 1
		}
		m.selection.endLine = line
		m.selection.endCol = col
		return true, m, nil

	case tea.MouseActionRelease:
		newM, cmd := m.finishSelection()
		return true, newM, cmd
	}
	return false, m, nil
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

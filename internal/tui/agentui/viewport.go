package agentui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/theme"
)

// popupHeight is the maximum number of terminal lines the TUI occupies.
// Inline mode (no alt screen) renders in place, so we cap the height to
// keep it feeling like a sized popup rather than a full-page takeover.
const popupHeight = 30

// inputRowHeight returns the row count of the input section between the
// viewport's bottom separator and the status line's top separator.
func (m Model) inputRowHeight() int {
	switch m.state {
	case stateApproval, stateCommandProposal:
		// The command block wraps to the viewport width and approval adds a
		// reason line, so both are variable height — measure rather than assume.
		if md := modeFor(m.state); md != nil {
			vpW, _ := m.viewportDims()
			return strings.Count(md.renderInputRow(m, vpW), "\n") + 1
		}
		return 4
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
// The bottom region is the status line normally, or a completion dropdown
// when one is visible.
func (m Model) totalViewHeight() int {
	_, vpH := m.viewportDims()
	bottomH := 1 // status line
	if m.state == stateIdle {
		if matches, _ := m.visibleDropdown(); len(matches) > 0 {
			bottomH = min(len(matches), dropdownMaxRows)
		}
	}
	return 3 + vpH + m.inputRowHeight() + bottomH
}

// viewportDims calculates viewport dimensions from terminal size.
//
// The height reserves five fixed rows — header, separator, one input row,
// separator, status — whatever the current state. The input section is often
// taller (multi-line input, pickers, command boxes) and so can the bottom row
// be (an open dropdown); the block then grows past the popup height rather
// than the viewport shrinking. totalViewHeight measures the real height.
func (m Model) viewportDims() (width, height int) {
	innerW := max(m.width, 10)
	h := popupHeight
	if m.height > 0 && m.height < h {
		h = m.height
	}
	innerH := max(h-5, 3)
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
	if m.mode == modeExplain {
		return dim.Render("  What command should I explain? Paste it below.")
	}
	return dim.Render("  Ready. Ask anything, or type ") +
		key.Render("/") +
		dim.Render(" for commands.")
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	vpW, vpH := m.viewportDims()
	if !m.ready {
		m.vp = viewport.New(viewport.WithWidth(vpW), viewport.WithHeight(vpH))
		m.ready = true
	} else {
		m.vp.SetWidth(vpW)
		m.vp.SetHeight(vpH)
	}
	m.input.SetWidth(vpW - 4)
	// glamour fixes the wrap width when the renderer is built, so a resize needs
	// a new one or the thread keeps wrapping to the old terminal.
	m.renderer = newRenderer(m.hasDarkBg, theme.Get(m.themeName), m.renderWidth())
	m.refreshViewport()
	return m, nil
}

// renderWidth is the column budget for rendered markdown: the viewport, less
// the gutter the thread indents entries by. Zero before the first
// WindowSizeMsg, which glamour reads as "do not wrap yet".
func (m Model) renderWidth() int {
	vpW, _ := m.viewportDims()
	if vpW <= threadGutter {
		return 0
	}
	return vpW - threadGutter
}

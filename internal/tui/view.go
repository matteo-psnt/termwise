package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	innerW, _ := m.viewportDims()

	// Conversation viewport.
	viewportView := m.vp.View()

	// Input / spinner / picker row.
	inputSection := m.renderInputSection(innerW)

	// Divider.
	divider := m.styles.Divider.Render(strings.Repeat("─", innerW))

	// Footer.
	footer := m.renderFooter()

	// Assemble inner content.
	inner := lipgloss.JoinVertical(lipgloss.Left,
		viewportView,
		inputSection,
		divider,
		footer,
	)

	// Wrap in rounded border.
	return m.styles.Outer.
		Width(m.width - 2).
		Render(inner)
}

func (m Model) renderInputSection(innerW int) string {
	switch m.state {
	case stateThinking:
		return m.styles.Spinner.Render(m.spin.View()) + " thinking..."

	case stateApproval:
		return m.styles.ApprovalHint.Render("  ↵ Approve   Esc Deny")

	case stateAskPicker:
		if m.activePicker != nil {
			return m.activePicker.View(m.styles)
		}
		return ""

	default: // stateIdle
		return m.styles.InputPrompt.Render("  > ") + m.input.View()
	}
}

func (m Model) renderFooter() string {
	tokens := m.inputTokens + m.outputTokens
	return m.styles.Footer.Render(
		fmt.Sprintf("  %s · %s · %s tokens",
			m.providerName,
			m.modelID,
			formatTokens(tokens),
		),
	)
}

func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}

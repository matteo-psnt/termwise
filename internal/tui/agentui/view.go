package agentui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if !m.ready {
		return "Loading...\n"
	}

	vpW, _ := m.viewportDims()

	// Build inner content: conversation + input line.
	inner := lipgloss.JoinVertical(lipgloss.Left,
		m.vp.View(),
		m.renderInputRow(),
	)

	// Wrap with 3-sided border (left │, right │, bottom ╰─╯).
	// Width is the content width before padding/borders.
	boxed := m.renderer.styles.Outer.Width(vpW).Render(inner)

	// Prepend the custom top border line with title.
	top := m.renderBoxTop(m.width)

	return lipgloss.JoinVertical(lipgloss.Left, top, boxed)
}

// renderBoxTop renders the top border of the box with model and token info:
//
//	╭─ termwise ──────────────── claude-haiku · 1,243 tok ─╮
//	╭─ termwise ──── ↓ 40% · claude-haiku · 1,243 tok ─╮  (when scrolled up)
func (m Model) renderBoxTop(w int) string {
	left := "─ termwise "
	tokens := formatTokens(m.inputTokens + m.outputTokens)
	modelInfo := m.modelID + " · " + tokens + " tok ─"
	var right string
	if m.userScrolled {
		pct := int(m.vp.ScrollPercent() * 100)
		right = fmt.Sprintf(" ↓ %d%% · %s", pct, modelInfo)
	} else {
		right = " " + modelInfo
	}

	inner := w - 2 // space for ╭ and ╮
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	fill := inner - leftW - rightW
	if fill < 1 {
		fill = 1
	}

	title := left + strings.Repeat("─", fill) + right
	return m.renderer.styles.Header.Render("╭" + title + "╮")
}

// renderInputRow renders the bottom input / status line inside the box.
func (m Model) renderInputRow() string {
	switch m.state {
	case stateThinking:
		return " " + m.renderer.styles.Spinner.Render(m.spin.View()) + " thinking..."

	case stateApproval:
		return m.renderer.styles.ApprovalHint.Render(" ↵ Approve   Esc Deny")

	case stateJudging:
		cmd, _ := m.pending.toolCall.Input["command"].(string)
		return " " + m.renderer.styles.Spinner.Render(m.spin.View()) + " " + cmd

	case stateAskPicker:
		if m.pending.picker != nil {
			return m.pending.picker.View(m.renderer)
		}
		return ""

	case stateHistSearch:
		query := m.histSearch.query
		count := len(m.histSearch.matches)
		hint := fmt.Sprintf(" (%d)", count)
		return m.renderer.styles.InputPrompt.Render(" / "+query+hint+" › ") + m.input.View()

	default: // stateIdle
		return m.renderer.styles.InputPrompt.Render(" › ") + m.input.View()
	}
}

func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}

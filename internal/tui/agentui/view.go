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
	sep := m.renderer.styles.Separator.Render(strings.Repeat("─", m.width))

	var inputSection string
	if m.showHelp {
		inputSection = lipgloss.JoinVertical(lipgloss.Left,
			sep,
			m.renderHelpOverlay(),
			sep,
		)
	} else {
		inputSection = lipgloss.JoinVertical(lipgloss.Left,
			sep,
			m.renderInputRow(vpW),
			sep,
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(m.width),
		m.vp.View(),
		inputSection,
		m.renderStatusLine(),
	)
}

// renderHeader renders the top title line.
func (m Model) renderHeader(w int) string {
	left := " termwise "
	var right string
	if m.userScrolled {
		pct := int(m.vp.ScrollPercent() * 100)
		right = fmt.Sprintf(" ↓ %d%% ", pct)
	}

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	fill := w - leftW - rightW
	if fill < 1 {
		fill = 1
	}

	title := left + strings.Repeat("─", fill) + right
	return m.renderer.styles.Header.Render(title)
}

// renderInputRow renders the bottom input area based on current state.
func (m Model) renderInputRow(vpW int) string {
	switch m.state {
	case stateThinking:
		return " " + m.renderer.styles.Spinner.Render(m.spin.View()) + " thinking..."

	case stateJudging:
		cmd, _ := m.pending.toolCall.Input["command"].(string)
		return " " + m.renderer.styles.Spinner.Render(m.spin.View()) + " " + cmd

	case stateApproval:
		cmd, _ := m.pending.toolCall.Input["command"].(string)
		return lipgloss.JoinVertical(lipgloss.Left,
			m.renderCommandBlock("run command?", cmd, vpW),
			m.renderer.styles.ActionHints.Render("  [↵] run   [esc] skip   [e] edit"),
		)

	case stateApprovalEdit:
		return m.renderer.styles.InputPrompt.Render(" ✎ ") + m.input.View()

	case stateCommandProposal:
		return lipgloss.JoinVertical(lipgloss.Left,
			m.renderCommandBlock("suggested command", "$ "+m.shellCommand, vpW),
			m.renderer.styles.ActionHints.Render("  [↵] accept   [esc] dismiss"),
		)

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
		input := m.input
		input.Placeholder = ""
		prompt := m.renderer.styles.InputPrompt.Render(" › ")
		var suggestion string
		if s := m.visibleSuggestion(); s != "" {
			suggestion = m.renderer.styles.Suggestion.Render(s)
		}
		return prompt + input.View() + suggestion
	}
}

// renderCommandBlock renders a titled thick-border box for bash approval and command proposals:
//
//	┏━ label ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
//	┃  content                                               ┃
//	┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
func (m Model) renderCommandBlock(label, content string, w int) string {
	style := m.renderer.styles.CommandBox
	inner := w - 2

	labelPart := "━ " + label + " "
	fillCount := inner - lipgloss.Width(labelPart)
	if fillCount < 1 {
		fillCount = 1
	}
	top := "┏" + labelPart + strings.Repeat("━", fillCount) + "┓"

	contentStr := "  " + content
	padCount := inner - lipgloss.Width(contentStr)
	if padCount < 0 {
		padCount = 0
	}
	mid := "┃" + contentStr + strings.Repeat(" ", padCount) + "┃"

	bot := "┗" + strings.Repeat("━", inner) + "┛"

	return style.Render(strings.Join([]string{top, mid, bot}, "\n"))
}

// renderHelpOverlay renders a contextual key-binding reference.
func (m Model) renderHelpOverlay() string {
	type binding struct{ key, desc string }

	var bindings []binding
	switch m.state {
	case stateThinking, stateJudging:
		bindings = []binding{
			{"esc", "interrupt"},
			{"?", "close help"},
			{"ctrl+c", "quit"},
		}
	case stateApproval, stateApprovalEdit:
		bindings = []binding{
			{"↵", "run command"},
			{"esc", "skip"},
			{"e", "edit command"},
			{"?", "close help"},
			{"ctrl+c", "quit"},
		}
	case stateCommandProposal:
		bindings = []binding{
			{"↵", "accept command"},
			{"esc", "dismiss"},
			{"?", "close help"},
			{"ctrl+c", "quit"},
		}
	default:
		bindings = []binding{
			{"↵", "submit"},
			{"tab", "accept suggestion"},
			{"↑ / ↓", "history"},
			{"ctrl+r", "search history"},
			{"?", "close help"},
			{"ctrl+c", "quit"},
		}
	}

	const keyColW = 14
	var sb strings.Builder
	for _, b := range bindings {
		key := m.renderer.styles.HelpKey.Render(b.key)
		desc := m.renderer.styles.HelpDesc.Render(b.desc)
		pad := keyColW - lipgloss.Width(b.key)
		if pad < 1 {
			pad = 1
		}
		sb.WriteString("  " + key + strings.Repeat(" ", pad) + desc + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// renderStatusLine renders a one-line status bar below the input separator:
//
//	termwise | Sonnet 4.6 [med] | 1,243 tok                  Esc again to clear
func (m Model) renderStatusLine() string {
	tokens := formatTokens(m.inputTokens + m.outputTokens)
	modelPart := m.modelShortName()
	if eff := m.effectiveEffort(); eff != "" {
		modelPart += " [" + effortLabel(eff) + "]"
	}
	left := fmt.Sprintf(" %s | %s | %s tok ", m.workDir, modelPart, tokens)
	leftStr := m.renderer.styles.ActionHints.Render(left)

	var rightStr string
	if !m.lastEscAt.IsZero() {
		rightStr = m.renderer.styles.ActionHints.Render("  Esc again to clear  ")
	}

	pad := m.width - lipgloss.Width(leftStr) - lipgloss.Width(rightStr)
	if pad < 0 {
		pad = 0
	}
	return leftStr + strings.Repeat(" ", pad) + rightStr
}

func effortLabel(effort string) string {
	switch effort {
	case "medium":
		return "med"
	default:
		return effort // "low", "high", "max" — already short
	}
}

func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}

package agentui

import (
	"fmt"
	"strings"
	"time"

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

	vpView := m.vp.View()
	if m.selection.has() {
		vpView = renderViewportWithSelection(vpView, m.vp.YOffset, m.selection)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(m.width),
		vpView,
		inputSection,
		m.renderStatusLine(),
	)
}

// renderHeader renders the top title line. The title is rendered in accent
// while the fill and scroll indicator stay dim for a cleaner visual hierarchy.
func (m Model) renderHeader(w int) string {
	title := m.renderer.styles.HeaderTitle.Render(" termwise ")
	var right string
	if m.userScrolled {
		pct := int(m.vp.ScrollPercent() * 100)
		right = m.renderer.styles.Header.Render(fmt.Sprintf(" ↓ %d%% ", pct))
	}

	titleW := lipgloss.Width(title)
	rightW := lipgloss.Width(right)
	fill := w - titleW - rightW
	if fill < 1 {
		fill = 1
	}
	return title + m.renderer.styles.Header.Render(strings.Repeat("─", fill)) + right
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

	case stateSlashPicker:
		if m.slashPicker != nil {
			return m.slashPicker.View(m.renderer)
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

		// Slash-command dropdown and inline ghost take precedence over prompt
		// suggestions whenever the dropdown is visible.
		matches := m.visibleSlashMatches()
		var suggestion string
		if len(matches) > 0 && m.slashCursor < len(matches) {
			suggestion = m.renderer.styles.Suggestion.Render(matches[m.slashCursor].Completion)
		} else if s := m.visibleSuggestion(); s != "" {
			suggestion = m.renderer.styles.Suggestion.Render(s)
		}

		line := prompt + input.View() + suggestion
		if len(matches) > 0 {
			return m.renderSlashDropdown(matches) + "\n" + line
		}
		return line
	}
}

// renderSlashDropdown renders the slash-command match list. Highlights the
// cursor row and scrolls a windowed view when matches exceed slashDropdownMaxRows.
const slashDropdownMaxRows = 5

func (m Model) renderSlashDropdown(matches []slashMatch) string {
	start := 0
	end := len(matches)
	if end > slashDropdownMaxRows {
		if m.slashCursor >= slashDropdownMaxRows {
			start = m.slashCursor - slashDropdownMaxRows + 1
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
		marker := "  "
		var labelOut string
		if start+i == m.slashCursor {
			marker = m.renderer.styles.InputPrompt.Render("› ")
			labelOut = m.renderer.styles.InputPrompt.Bold(true).Render(mt.Label)
		} else {
			labelOut = mt.Label
		}
		b.WriteString(marker + labelOut)
		if mt.Description != "" {
			pad := labelW - lipgloss.Width(mt.Label) + 2
			if pad < 1 {
				pad = 1
			}
			b.WriteString(strings.Repeat(" ", pad) + m.renderer.styles.ActionHints.Render(mt.Description))
		}
		if i < len(visible)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// renderCommandBlock renders a titled rounded-border box for bash approval
// and command proposals:
//
//	╭─ label ────────────────────────────────────────────────╮
//	│  content                                               │
//	╰────────────────────────────────────────────────────────╯
func (m Model) renderCommandBlock(label, content string, w int) string {
	style := m.renderer.styles.CommandBox
	inner := w - 2

	labelPart := "─ " + label + " "
	fillCount := inner - lipgloss.Width(labelPart)
	if fillCount < 1 {
		fillCount = 1
	}
	top := "╭" + labelPart + strings.Repeat("─", fillCount) + "╮"

	contentStr := "  " + content
	padCount := inner - lipgloss.Width(contentStr)
	if padCount < 0 {
		padCount = 0
	}
	mid := "│" + contentStr + strings.Repeat(" ", padCount) + "│"

	bot := "╰" + strings.Repeat("─", inner) + "╯"

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
			{"/", "slash commands (/help)"},
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
//	termwise · Sonnet 4.6 [med] · 1,243 tok                  Esc again to clear
func (m Model) renderStatusLine() string {
	tokens := formatTokens(m.inputTokens + m.outputTokens)
	modelPart := m.modelShortName()
	if eff := m.effectiveEffort(); eff != "" {
		modelPart += " [" + effortLabel(eff) + "]"
	}

	hints := m.renderer.styles.ActionHints
	sep := hints.Render(" · ")
	leftStr := hints.Render(" "+m.workDir) +
		sep + m.renderer.styles.StatusModel.Render(modelPart) +
		sep + hints.Render(tokens+" tok ")

	var rightStr string
	if !m.copyToastUntil.IsZero() && time.Now().Before(m.copyToastUntil) {
		rightStr = m.renderer.styles.StatusModel.Render("  ✓ " + m.copyToastMsg + "  ")
	} else if !m.lastEscAt.IsZero() {
		rightStr = hints.Render("  Esc again to clear  ")
	}

	pad := m.width - lipgloss.Width(leftStr) - lipgloss.Width(rightStr)
	if pad < 0 {
		pad = 0
	}
	return leftStr + strings.Repeat(" ", pad) + rightStr
}

func effortLabel(effort string) string {
	if effort == "medium" {
		return "med"
	}
	return effort
}

func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}

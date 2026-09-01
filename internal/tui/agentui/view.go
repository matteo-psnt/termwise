package agentui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// View implements tea.Model. MouseMode is set per-frame (v2 moved mouse and
// keyboard setup off program options onto the view); basic Kitty keyboard
// disambiguation — which delivers a distinct Shift+Enter on capable terminals —
// is enabled by bubbletea automatically.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// render composes the TUI string for the current state.
func (m Model) render() string {
	if m.quitting {
		return ""
	}
	if !m.ready {
		return "Loading...\n"
	}
	view := m.renderedView()
	if m.selection.has() {
		view = applySelectionToView(view, m.selection)
	}
	return view
}

// renderedView composes the TUI block without selection highlight. Used by
// both View() and the copy path so they share the same row layout.
func (m Model) renderedView() string {
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
	// The slash dropdown takes the status line's spot while it's open, so the
	// list of commands sits flush under the bottom separator.
	bottom := m.renderStatusLine()
	if m.state == stateIdle {
		if matches := m.visibleSlashMatches(); len(matches) > 0 {
			bottom = m.renderSlashDropdown(matches)
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(m.width),
		m.vp.View(),
		inputSection,
		bottom,
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
	fill := max(w-titleW-rightW, 1)
	return title + m.renderer.styles.Header.Render(strings.Repeat("─", fill)) + right
}

// renderInputRow delegates to the active mode for the bottom input area.
func (m Model) renderInputRow(vpW int) string {
	md := modeFor(m.state)
	if md == nil {
		return ""
	}
	return md.renderInputRow(m, vpW)
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
	fillCount := max(inner-lipgloss.Width(labelPart), 1)
	top := "╭" + labelPart + strings.Repeat("─", fillCount) + "╮"

	contentStr := "  " + content
	padCount := max(inner-lipgloss.Width(contentStr), 0)
	mid := "│" + contentStr + strings.Repeat(" ", padCount) + "│"

	bot := "╰" + strings.Repeat("─", inner) + "╯"

	return style.Render(strings.Join([]string{top, mid, bot}, "\n"))
}

// renderHelpOverlay renders a contextual key-binding reference. The active
// mode supplies its own binding list.
func (m Model) renderHelpOverlay() string {
	md := modeFor(m.state)
	if md == nil {
		return ""
	}
	// ?/ctrl+c are handled globally in Model.handleKey, so they're appended here
	// once rather than repeated in every mode's binding list.
	bindings := append(md.helpBindings(m),
		binding{"?", "close help"},
		binding{"ctrl+c", "quit"},
	)

	const keyColW = 14
	var sb strings.Builder
	for _, b := range bindings {
		key := m.renderer.styles.HelpKey.Render(b.key)
		desc := m.renderer.styles.HelpDesc.Render(b.desc)
		pad := max(keyColW-lipgloss.Width(b.key), 1)
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
		rightStr = m.renderer.styles.CopyToast.Render(" ✓ " + m.copyToastMsg + " ")
	} else if !m.lastEscAt.IsZero() {
		rightStr = hints.Render("  Esc again to clear  ")
	}

	pad := max(m.width-lipgloss.Width(leftStr)-lipgloss.Width(rightStr), 0)
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

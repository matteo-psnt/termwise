package configui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteo-psnt/termwise/internal/config"
)

func (m editorModel) View() tea.View {
	box := m.styles.Outer.Render(m.renderInner())
	content := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m editorModel) renderInner() string {
	header := m.styles.Title.Render("termwise config") + "\n\n"
	switch {
	case m.modelPicker != nil:
		return header + m.modelPicker.View()
	case m.authEditor != nil:
		return header + m.authEditor.View()
	case m.keybinding != nil:
		return header + m.keybinding.View()
	case m.themePicker != nil:
		return header + m.themePicker.View(m.styles, m.hasDarkBg)
	case m.enumPicker != nil:
		return header + m.enumPicker.View()
	case m.addWizard != nil:
		return m.addWizard.renderInner()
	default:
		return m.renderNormal()
	}
}

func (m editorModel) renderNormal() string {
	// Rows are rendered into their own builder first and split into lines,
	// so the list can be windowed by screen line: section headers take two.
	var b, list strings.Builder
	cursorLine := 0

	for i, row := range m.rows {
		focused := i == m.cursor
		if focused {
			cursorLine = strings.Count(list.String(), "\n")
		}

		switch row.kind {
		case rowSectionHeader:
			if i > 0 {
				list.WriteString("\n")
			}
			list.WriteString("  " + m.styles.Title.Render(row.label) + "\n")

		case rowProvHeader:
			prefix := m.rowPrefix(focused)
			indicator := ""
			if row.provider == m.cfg.SelectedProvider {
				indicator = " " + m.connIndicator()
			}
			if focused {
				list.WriteString(prefix + m.styles.Selected.Render(row.provider) + indicator + "\n")
			} else {
				list.WriteString(prefix + row.provider + indicator + "\n")
			}

		case rowProvModel:
			val := m.cfg.Providers[row.provider].Model
			if val == "" {
				val = "(none)"
			}
			m.renderRow(&list, focused, "     Model   ", val)

		case rowProvEffort:
			m.renderRow(&list, focused, "     Effort  ", displayEffort(m.cfg.Providers[row.provider].Effort))

		case rowProvAuth:
			m.renderRow(&list, focused, "     Auth    ", describeAuth(row.provider, m.cfg.Providers[row.provider]))

		case rowAddProvider:
			prefix := m.rowPrefix(focused)
			if focused {
				list.WriteString(prefix + m.styles.Selected.Render("+ Add provider") + "\n")
			} else {
				list.WriteString(prefix + m.styles.Dim.Render("+ Add provider") + "\n")
			}

		case rowSetting:
			def := config.Settings[row.settingIdx]
			m.renderRow(&list, focused, fmt.Sprintf("%-*s", settingLabelWidth(), def.Label), displayValue(def, m.cfg))
		}
	}

	var footer string
	if len(m.rows) > 0 && m.cursor < len(m.rows) {
		row := m.rows[m.cursor]
		width := m.footerWidth()
		footer = m.styles.Dim.Render(padToWidth(row.desc, width)) + "\n" +
			m.styles.Dim.Render(padToWidth(m.effectiveHint(row), width))
	}

	b.WriteString(m.windowList(list.String(), cursorLine, lipgloss.Height(footer)))
	b.WriteString("\n" + footer)
	return b.String()
}

// windowList cuts the rendered list down to the lines that fit on screen
// above a footer footerLines tall, scrolled to keep cursorLine visible. Short
// terminals otherwise lose the bottom rows, and with them the footer, off the
// screen.
func (m editorModel) windowList(list string, cursorLine, footerLines int) string {
	lines := strings.Split(strings.TrimSuffix(list, "\n"), "\n")
	// The box's border, the blank line under the list, and the footer.
	chrome := 2 + 1 + footerLines
	if m.height <= 0 || len(lines) <= m.height-chrome {
		return list
	}
	// Two more for the scroll markers, which are always drawn while
	// scrolling (blank when nothing is cut off) so the box keeps its height.
	size := max(m.height-chrome-2, 3)
	start, end := listWindow(len(lines), cursorLine, size)
	var b strings.Builder
	if start > 0 {
		b.WriteString(m.styles.Dim.Render("  ↑ more"))
	}
	b.WriteString("\n")
	for _, line := range lines[start:end] {
		b.WriteString(line + "\n")
	}
	if end < len(lines) {
		b.WriteString(m.styles.Dim.Render("  ↓ more"))
	}
	b.WriteString("\n")
	return b.String()
}

// effectiveHint returns the keyboard hint for a row, swapping in a contextual
// message for rows whose primary action depends on current state — e.g. the
// "set active" hint disappears on the already-active provider.
func (m editorModel) effectiveHint(row editorRow) string {
	if row.kind == rowProvHeader && row.provider == m.cfg.SelectedProvider {
		return navTail
	}
	return row.hint
}

// footerWidth returns the column width that desc/hint lines are padded to.
// Padding both to the same constant width keeps the box's auto-sized width
// stable as the cursor moves between rows.
func (m editorModel) footerWidth() int {
	w := 0
	for _, row := range m.rows {
		if n := lipgloss.Width(row.desc); n > w {
			w = n
		}
		if n := lipgloss.Width(row.hint); n > w {
			w = n
		}
	}
	return w
}

func padToWidth(s string, width int) string {
	if pad := width - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

func (m editorModel) rowPrefix(focused bool) string {
	if focused {
		return m.styles.Selected.Render("▶ ")
	}
	return "  "
}

// renderRow renders a labeled value row. label should already be padded/formatted
// for display (e.g. "     Model   " for sub-rows, or padded to settingLabelWidth
// for settings).
func (m editorModel) renderRow(b *strings.Builder, focused bool, label, value string) {
	prefix := m.rowPrefix(focused)
	if focused {
		b.WriteString(prefix + m.styles.Selected.Render(label+value) + "\n")
	} else {
		b.WriteString(prefix + m.styles.Dim.Render(label) + value + "\n")
	}
}

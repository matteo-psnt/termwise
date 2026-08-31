package configui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/matteo-psnt/termwise/internal/config"
)

func (m editorModel) View() string {
	box := m.styles.Outer.Render(m.renderInner())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
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
		return header + m.themePicker.View()
	case m.enumPicker != nil:
		return header + m.enumPicker.View()
	case m.addWizard != nil:
		return m.addWizard.renderInner()
	default:
		return m.renderNormal()
	}
}

func (m editorModel) renderNormal() string {
	var b strings.Builder

	for i, row := range m.rows {
		focused := i == m.cursor

		switch row.kind {
		case rowSectionHeader:
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString("  " + m.styles.Title.Render(row.label) + "\n")

		case rowProvHeader:
			prefix := m.rowPrefix(focused)
			indicator := ""
			if row.provider == m.cfg.SelectedProvider {
				indicator = " " + m.connIndicator()
			}
			if focused {
				b.WriteString(prefix + m.styles.Selected.Render(row.provider) + indicator + "\n")
			} else {
				b.WriteString(prefix + row.provider + indicator + "\n")
			}

		case rowProvModel:
			val := m.cfg.Providers[row.provider].Model
			if val == "" {
				val = "(none)"
			}
			m.renderRow(&b, focused, "     Model   ", val)

		case rowProvEffort:
			m.renderRow(&b, focused, "     Effort  ", displayEffort(m.cfg.Providers[row.provider].Effort))

		case rowProvAuth:
			m.renderRow(&b, focused, "     Auth    ", describeAuth(row.provider, m.cfg.Providers[row.provider]))

		case rowAddProvider:
			prefix := m.rowPrefix(focused)
			if focused {
				b.WriteString(prefix + m.styles.Selected.Render("+ Add provider") + "\n")
			} else {
				b.WriteString(prefix + m.styles.Dim.Render("+ Add provider") + "\n")
			}

		case rowSetting:
			def := config.Settings[row.settingIdx]
			m.renderRow(&b, focused, fmt.Sprintf("%-*s", settingLabelWidth(), def.Label), displayValue(def, m.cfg))
		}
	}

	b.WriteString("\n")
	if len(m.rows) > 0 && m.cursor < len(m.rows) {
		row := m.rows[m.cursor]
		width := m.footerWidth()
		b.WriteString(m.styles.Dim.Render(padToWidth(row.desc, width)) + "\n")
		b.WriteString(m.styles.Dim.Render(padToWidth(m.effectiveHint(row), width)))
	}

	return b.String()
}

// effectiveHint returns the keyboard hint for a row, swapping in a contextual
// message for rows whose primary action depends on current state — e.g. the
// "set active" hint disappears on the already-active provider.
func (m editorModel) effectiveHint(row editorRow) string {
	if row.kind == rowProvHeader && row.provider == m.cfg.SelectedProvider {
		return "↑/↓ navigate   esc revert   q quit"
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
// for display (e.g. "     Model   " for sub-rows, fmt.Sprintf("%-13s", ...) for settings).
func (m editorModel) renderRow(b *strings.Builder, focused bool, label, value string) {
	prefix := m.rowPrefix(focused)
	if focused {
		b.WriteString(prefix + m.styles.Selected.Render(label+value) + "\n")
	} else {
		b.WriteString(prefix + m.styles.Dim.Render(label) + value + "\n")
	}
}

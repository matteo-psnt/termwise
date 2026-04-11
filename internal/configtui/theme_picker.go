package configtui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/matteo-psnt/termwise/internal/theme"
)

type themePickerModel struct {
	styles configStyles
	r      *lipgloss.Renderer
	cursor int
	prev   string // original theme name, restored on cancel

	result    string
	done      bool
	cancelled bool
}

func newThemePicker(current string, r *lipgloss.Renderer, styles configStyles) themePickerModel {
	return themePickerModel{
		styles: styles,
		r:      r,
		cursor: themeIndex(current),
		prev:   theme.Normalize(current),
	}
}

// PreviewName returns the theme name currently under the cursor.
func (m themePickerModel) PreviewName() string {
	palettes := theme.All()
	if m.cursor >= 0 && m.cursor < len(palettes) {
		return palettes[m.cursor].Name
	}
	return m.prev
}

func (m themePickerModel) Init() tea.Cmd { return nil }

func (m themePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	palettes := theme.All()
	switch key.String() {
	case "esc", "b":
		m.cancelled = true
		m.done = true
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.styles = newStylesForTheme(m.r, palettes[m.cursor].Name)
		}
	case "down", "j":
		if m.cursor < len(palettes)-1 {
			m.cursor++
			m.styles = newStylesForTheme(m.r, palettes[m.cursor].Name)
		}
	case "enter", " ":
		m.result = palettes[m.cursor].Name
		m.done = true
	}
	return m, nil
}

func (m themePickerModel) View() string {
	palettes := theme.All()
	selected := theme.Normalize(m.prev) // show badge on original selection

	var b strings.Builder
	b.WriteString("Theme palette:\n\n")
	for i, p := range palettes {
		line := paletteSwatches(m.r, p) + " " + p.Label
		if p.Name == selected {
			line += " " + m.styles.Dim.Render("(selected)")
		}
		if i == m.cursor {
			b.WriteString(m.styles.Selected.Render("▶ ") + line + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(renderThemePreview(m.r, palettes[m.cursor]))
	b.WriteString("\n\n" + m.styles.Dim.Render("↑/↓ preview   enter select   esc cancel"))
	return b.String()
}

func themeIndex(name string) int {
	name = theme.Normalize(name)
	for i, p := range theme.All() {
		if p.Name == name {
			return i
		}
	}
	return 0
}

func paletteSwatches(r *lipgloss.Renderer, palette theme.Palette) string {
	colors := []lipgloss.AdaptiveColor{
		palette.Accent,
		palette.Success,
		palette.Error,
		palette.Border,
	}
	var out strings.Builder
	for _, c := range colors {
		out.WriteString(r.NewStyle().Foreground(c).Render("●"))
		out.WriteString(" ")
	}
	return strings.TrimSpace(out.String())
}

func renderThemePreview(r *lipgloss.Renderer, palette theme.Palette) string {
	accent := r.NewStyle().Foreground(palette.Accent).Bold(true)
	success := r.NewStyle().Foreground(palette.Success)
	errorStyle := r.NewStyle().Foreground(palette.Error)
	muted := r.NewStyle().Foreground(palette.Muted)
	box := r.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(palette.Border).
		Padding(0, 1)

	var b strings.Builder
	b.WriteString(accent.Render("Preview: ") + palette.Label + "\n")
	b.WriteString("tw: ")
	b.WriteString(accent.Render("rg \"theme\" internal") + "\n")
	b.WriteString(success.Render("connected") + "  ")
	b.WriteString(errorStyle.Render("error") + "  ")
	b.WriteString(muted.Render("footer · 12,481 tok"))
	return box.Render(b.String())
}

package configui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteo-psnt/termwise/internal/theme"
)

// themePickerModel has no styles of its own: the editor restyles itself for
// the theme under the cursor (see updateThemePicker), and View draws with
// those, so the picker is always shown in the palette it is previewing.
type themePickerModel struct {
	cursor int
	prev   string // original theme name, restored on cancel

	result    string
	done      bool
	cancelled bool
}

func newThemePicker(current string) themePickerModel {
	return themePickerModel{
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

func (themePickerModel) Init() tea.Cmd { return nil }

func (m themePickerModel) Update(msg tea.Msg) (themePickerModel, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	palettes := theme.All()
	if c, ok := moveListCursor(key.String(), m.cursor, len(palettes)); ok {
		m.cursor = c
		return m, nil
	}
	switch key.String() {
	case "esc", "b":
		m.cancelled = true
		m.done = true
	case "enter", " ":
		m.result = m.PreviewName()
		m.done = true
	}
	return m, nil
}

func (m themePickerModel) View(styles configStyles, hasDarkBg bool) string {
	palettes := theme.All()
	selected := m.prev // show badge on original selection

	var b strings.Builder
	b.WriteString("Theme palette:\n\n")
	for i, p := range palettes {
		line := paletteSwatches(hasDarkBg, p) + " " + p.Label
		if p.Name == selected {
			line += " " + styles.Dim.Render("(selected)")
		}
		if i == m.cursor {
			b.WriteString(styles.Selected.Render("▶ ") + line + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(renderThemePreview(hasDarkBg, theme.Get(m.PreviewName())))
	b.WriteString("\n\n" + styles.Dim.Render("↑/↓ preview   enter select   esc cancel"))
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

func paletteSwatches(hasDarkBg bool, palette theme.Palette) string {
	resolve := resolver(hasDarkBg)
	colors := []theme.Color{
		palette.Accent,
		palette.Success,
		palette.Error,
		palette.Border,
	}
	var out strings.Builder
	for _, c := range colors {
		out.WriteString(lipgloss.NewStyle().Foreground(resolve(c)).Render("●"))
		out.WriteString(" ")
	}
	return strings.TrimSpace(out.String())
}

func renderThemePreview(hasDarkBg bool, palette theme.Palette) string {
	resolve := resolver(hasDarkBg)
	accent := lipgloss.NewStyle().Foreground(resolve(palette.Accent)).Bold(true)
	success := lipgloss.NewStyle().Foreground(resolve(palette.Success))
	errorStyle := lipgloss.NewStyle().Foreground(resolve(palette.Error))
	muted := lipgloss.NewStyle().Foreground(resolve(palette.Muted))
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(resolve(palette.Border)).
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

package configui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/matteo-psnt/termwise/internal/theme"
)

type configStyles struct {
	Outer    lipgloss.Style
	Title    lipgloss.Style
	Selected lipgloss.Style
	Normal   lipgloss.Style
	Dim      lipgloss.Style
	Success  lipgloss.Style
	Error    lipgloss.Style
	Hint     lipgloss.Style
	Spinner  lipgloss.Style
}

func newStyles(r *lipgloss.Renderer) configStyles {
	return newStylesForTheme(r, theme.DefaultName)
}

func newStylesForTheme(r *lipgloss.Renderer, themeName string) configStyles {
	palette := theme.Get(themeName)
	accent := palette.Accent
	green := palette.Success
	red := palette.Error
	border := palette.Border

	return configStyles{
		Outer:    r.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1),
		Title:    r.NewStyle().Bold(true),
		Selected: r.NewStyle().Foreground(accent).Bold(true),
		Normal:   r.NewStyle(),
		Dim:      r.NewStyle().Faint(true),
		Success:  r.NewStyle().Foreground(green),
		Error:    r.NewStyle().Foreground(red),
		Hint:     r.NewStyle().Faint(true).Italic(true),
		Spinner:  r.NewStyle().Foreground(accent),
	}
}

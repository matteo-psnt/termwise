package configui

import (
	"image/color"

	"charm.land/lipgloss/v2"

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

func newStylesForTheme(hasDarkBg bool, themeName string) configStyles {
	palette := theme.Get(themeName)
	ld := lipgloss.LightDark(hasDarkBg)
	resolve := func(c theme.Color) color.Color {
		return ld(lipgloss.Color(c.Light), lipgloss.Color(c.Dark))
	}
	accent := resolve(palette.Accent)
	green := resolve(palette.Success)
	red := resolve(palette.Error)
	border := resolve(palette.Border)

	return configStyles{
		Outer:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1),
		Title:    lipgloss.NewStyle().Bold(true),
		Selected: lipgloss.NewStyle().Foreground(accent).Bold(true),
		Normal:   lipgloss.NewStyle(),
		Dim:      lipgloss.NewStyle().Faint(true),
		Success:  lipgloss.NewStyle().Foreground(green),
		Error:    lipgloss.NewStyle().Foreground(red),
		Hint:     lipgloss.NewStyle().Faint(true).Italic(true),
		Spinner:  lipgloss.NewStyle().Foreground(accent),
	}
}

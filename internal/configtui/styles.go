package configtui

import "github.com/charmbracelet/lipgloss"

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
	accent := lipgloss.AdaptiveColor{Light: "#5B8DD9", Dark: "#7AA2F7"}
	green  := lipgloss.AdaptiveColor{Light: "#27AE60", Dark: "#98C379"}
	red    := lipgloss.AdaptiveColor{Light: "#E06C75", Dark: "#FF5F87"}
	border := lipgloss.AdaptiveColor{Light: "#888888", Dark: "#555555"}

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

package tui

import "github.com/charmbracelet/lipgloss"

// glamourStyle returns "dark" or "light" based on the renderer's cached
// background detection, avoiding any additional OSC 11 terminal queries.
func glamourStyle(r *lipgloss.Renderer) string {
	if r.HasDarkBackground() {
		return "dark"
	}
	return "light"
}

// Styles holds all lipgloss styles for the TUI.
// Built once at model construction from a renderer tied to the output TTY.
type Styles struct {
	Outer        lipgloss.Style
	Header       lipgloss.Style
	UserPrefix   lipgloss.Style
	TWPrefix     lipgloss.Style
	ToolName     lipgloss.Style
	ToolAuto     lipgloss.Style
	ToolOutput   lipgloss.Style
	ApprovalHint lipgloss.Style
	Footer       lipgloss.Style
	Spinner      lipgloss.Style
	Error        lipgloss.Style
	Command      lipgloss.Style
	InputPrompt  lipgloss.Style
}

func newStyles(r *lipgloss.Renderer) Styles {
	accent := lipgloss.AdaptiveColor{Light: "#5B8DD9", Dark: "#7AA2F7"}
	errCol := lipgloss.AdaptiveColor{Light: "#E06C75", Dark: "#FF5F87"}
	dim    := lipgloss.AdaptiveColor{Light: "#AAAAAA", Dark: "#555555"}

	return Styles{
		// 3-sided rounded box: left │, right │, bottom ╰─╯. Top is rendered manually.
		Outer: r.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderLeft(true).
			BorderRight(true).
			BorderBottom(true).
			BorderTop(false).
			BorderForeground(dim),
		Header:       r.NewStyle().Foreground(dim),
		UserPrefix:   r.NewStyle().Faint(true),
		TWPrefix:     r.NewStyle().Foreground(accent),
		ToolName:     r.NewStyle().Faint(true),
		ToolAuto:     r.NewStyle().Faint(true).Italic(true),
		ToolOutput:   r.NewStyle().Faint(true),
		ApprovalHint: r.NewStyle().Faint(true),
		Footer:       r.NewStyle().Faint(true),
		Spinner:      r.NewStyle().Foreground(accent),
		Error:        r.NewStyle().Foreground(errCol),
		Command:      r.NewStyle().Bold(true),
		InputPrompt:  r.NewStyle().Foreground(accent),
	}
}

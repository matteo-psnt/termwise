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
	UserPrefix   lipgloss.Style
	TWPrefix     lipgloss.Style
	ToolName     lipgloss.Style
	ToolAuto     lipgloss.Style
	ToolOutput   lipgloss.Style
	ApprovalHint lipgloss.Style
	Divider      lipgloss.Style
	Footer       lipgloss.Style
	Spinner      lipgloss.Style
	Error        lipgloss.Style
	Command      lipgloss.Style
	InputPrompt  lipgloss.Style
}

func newStyles(r *lipgloss.Renderer) Styles {
	border  := lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"}
	accent  := lipgloss.AdaptiveColor{Light: "#5B8DD9", Dark: "#7AA2F7"}
	errCol  := lipgloss.AdaptiveColor{Light: "#E06C75", Dark: "#FF5F87"}
	dim     := lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"}

	return Styles{
		Outer: r.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border),

		UserPrefix:   r.NewStyle().Faint(true),
		TWPrefix:     r.NewStyle().Foreground(accent),
		ToolName:     r.NewStyle().Faint(true),
		ToolAuto:     r.NewStyle().Faint(true).Italic(true),
		ToolOutput:   r.NewStyle().Faint(true),
		ApprovalHint: r.NewStyle().Faint(true),
		Divider:      r.NewStyle().Foreground(dim),
		Footer:       r.NewStyle().Faint(true),
		Spinner:      r.NewStyle().Foreground(accent),
		Error:        r.NewStyle().Foreground(errCol),
		Command:      r.NewStyle().Bold(true),
		InputPrompt:  r.NewStyle().Faint(true),
	}
}

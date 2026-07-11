package agentui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/matteo-psnt/termwise/internal/theme"
)

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
	Header      lipgloss.Style
	Separator   lipgloss.Style
	UserSymbol  lipgloss.Style
	TWSymbol    lipgloss.Style
	ToolCall    lipgloss.Style
	ToolOutput  lipgloss.Style
	Spinner     lipgloss.Style
	Error       lipgloss.Style
	Command     lipgloss.Style
	InputPrompt lipgloss.Style
	Suggestion  lipgloss.Style
	CommandBox  lipgloss.Style
	ActionHints lipgloss.Style
	HelpKey     lipgloss.Style
	HelpDesc    lipgloss.Style
}

func newStyles(r *lipgloss.Renderer, palette theme.Palette) Styles {
	accent := palette.Accent
	errCol := palette.Error
	dim := palette.Muted

	return Styles{
		Header:      r.NewStyle().Foreground(dim),
		Separator:   r.NewStyle().Foreground(dim),
		UserSymbol:  r.NewStyle().Faint(true),
		TWSymbol:    r.NewStyle().Foreground(accent),
		ToolCall:    r.NewStyle().Faint(true),
		ToolOutput:  r.NewStyle().Foreground(dim),
		Spinner:     r.NewStyle().Foreground(accent),
		Error:       r.NewStyle().Foreground(errCol),
		Command:     r.NewStyle().Bold(true),
		InputPrompt: r.NewStyle().Foreground(accent),
		Suggestion:  r.NewStyle().Foreground(dim).Italic(true),
		CommandBox:  r.NewStyle().Foreground(accent),
		ActionHints: r.NewStyle().Faint(true),
		HelpKey:     r.NewStyle().Foreground(accent).Bold(true),
		HelpDesc:    r.NewStyle().Faint(true),
	}
}

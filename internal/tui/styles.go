package tui

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
	Outer        lipgloss.Style
	Header       lipgloss.Style
	UserSymbol   lipgloss.Style
	TWSymbol     lipgloss.Style
	ToolCall     lipgloss.Style
	ToolOutput   lipgloss.Style
	ApprovalHint lipgloss.Style
	Footer       lipgloss.Style
	Spinner      lipgloss.Style
	Error        lipgloss.Style
	Command      lipgloss.Style
	InputPrompt  lipgloss.Style
}

func newStyles(r *lipgloss.Renderer, palette theme.Palette) Styles {
	accent := palette.Accent
	errCol := palette.Error
	dim := palette.Muted

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
		UserSymbol:   r.NewStyle().Faint(true),
		TWSymbol:     r.NewStyle().Foreground(accent),
		ToolCall:     r.NewStyle().Faint(true),
		ToolOutput:   r.NewStyle().Faint(true),
		ApprovalHint: r.NewStyle().Faint(true),
		Footer:       r.NewStyle().Faint(true),
		Spinner:      r.NewStyle().Foreground(accent),
		Error:        r.NewStyle().Foreground(errCol),
		Command:      r.NewStyle().Bold(true),
		InputPrompt:  r.NewStyle().Foreground(accent),
	}
}

package agentui

import (
	"image/color"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"

	"github.com/matteo-psnt/termwise/internal/theme"
)

// inputStyles builds the textarea styling for the prompt: a static
// (non-blinking) cursor and the given placeholder style, in both focused and
// blurred states. The cursor-line background is cleared so the multi-line input
// doesn't paint a highlighted strip across the row.
func inputStyles(hasDarkBg bool, placeholder lipgloss.Style) textarea.Styles {
	st := textarea.DefaultStyles(hasDarkBg)
	st.Cursor.Blink = false
	st.Focused.Placeholder = placeholder
	st.Blurred.Placeholder = placeholder
	st.Focused.CursorLine = lipgloss.NewStyle()
	st.Blurred.CursorLine = lipgloss.NewStyle()
	return st
}

// Styles holds all lipgloss styles for the TUI.
// Built once at model construction from the detected terminal background.
type Styles struct {
	Header      lipgloss.Style
	HeaderTitle lipgloss.Style
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
	StatusModel lipgloss.Style
	CopyToast   lipgloss.Style
	HelpKey     lipgloss.Style
	HelpDesc    lipgloss.Style
}

func newStyles(hasDarkBg bool, palette theme.Palette) Styles {
	ld := lipgloss.LightDark(hasDarkBg)
	resolve := func(c theme.Color) color.Color {
		return ld(lipgloss.Color(c.Light), lipgloss.Color(c.Dark))
	}
	accent := resolve(palette.Accent)
	errCol := resolve(palette.Error)
	dim := resolve(palette.Muted)

	return Styles{
		Header:      lipgloss.NewStyle().Foreground(dim),
		HeaderTitle: lipgloss.NewStyle().Foreground(accent).Bold(true),
		Separator:   lipgloss.NewStyle().Foreground(dim),
		UserSymbol:  lipgloss.NewStyle().Faint(true),
		TWSymbol:    lipgloss.NewStyle().Foreground(accent),
		ToolCall:    lipgloss.NewStyle().Faint(true),
		ToolOutput:  lipgloss.NewStyle().Foreground(dim),
		Spinner:     lipgloss.NewStyle().Foreground(accent),
		Error:       lipgloss.NewStyle().Foreground(errCol),
		Command:     lipgloss.NewStyle().Bold(true),
		InputPrompt: lipgloss.NewStyle().Foreground(accent),
		Suggestion:  lipgloss.NewStyle().Foreground(dim).Italic(true),
		CommandBox:  lipgloss.NewStyle().Foreground(accent),
		ActionHints: lipgloss.NewStyle().Faint(true),
		StatusModel: lipgloss.NewStyle().Foreground(accent),
		CopyToast:   lipgloss.NewStyle().Foreground(accent).Bold(true).Reverse(true),
		HelpKey:     lipgloss.NewStyle().Foreground(accent).Bold(true),
		HelpDesc:    lipgloss.NewStyle().Faint(true),
	}
}

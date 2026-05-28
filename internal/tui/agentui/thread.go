package agentui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteo-psnt/termwise/internal/theme"
)

// Renderer holds the config needed to render thread entries.
// Passed as a value to each entry's render method, keeping rendering stateless
// with all presentation config in one place.
type Renderer struct {
	styles  Styles
	glamour *glamour.TermRenderer
}

func newRenderer(r *lipgloss.Renderer, palette theme.Palette, glamourStyle string) Renderer {
	var cfg ansi.StyleConfig
	if glamourStyle == "dark" {
		cfg = styles.DarkStyleConfig
	} else {
		cfg = styles.LightStyleConfig
	}
	var zero uint
	cfg.Document.Margin = &zero

	gr, _ := glamour.NewTermRenderer(glamour.WithStyles(cfg), glamour.WithWordWrap(0))
	return Renderer{
		styles:  newStyles(r, palette),
		glamour: gr,
	}
}

// RenderThread renders all thread entries joined by newlines.
func (r Renderer) RenderThread(entries []ThreadEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(e.render(r))
	}
	return b.String()
}

// ThreadEntry is any displayable item in the conversation thread.
type ThreadEntry interface {
	render(r Renderer) string
}

// UserEntry is a message typed by the user.
type UserEntry struct{ Content string }

func (e UserEntry) render(r Renderer) string {
	return r.styles.UserSymbol.Render("› ") + e.Content
}

// AssistantEntry is text output from the model (direct or via the respond tool).
type AssistantEntry struct{ Content string }

func (e AssistantEntry) render(r Renderer) string {
	text := strings.TrimLeft(renderMarkdown(e.Content, r.glamour), "\n")
	return r.styles.TWSymbol.Render("◆ ") + text
}

// ToolCallEntry shows a tool being invoked.
type ToolCallEntry struct {
	Name   string // e.g. "bash"
	Detail string // e.g. the command or path
}

func (e ToolCallEntry) render(r Renderer) string {
	return r.styles.ToolCall.Render(capitalizeFirst(e.Name) + "(" + e.Detail + ")")
}

// ToolResultEntry shows the output of a tool call.
type ToolResultEntry struct {
	Content string
	IsError bool
}

const maxDisplayLines = 20

func (e ToolResultEntry) render(r Renderer) string {
	lines := strings.Split(strings.TrimRight(e.Content, "\n"), "\n")
	total := len(lines)

	display := lines
	truncated := 0
	if total > maxDisplayLines {
		display = lines[:maxDisplayLines]
		truncated = total - maxDisplayLines
	}

	style := r.styles.ToolOutput
	if e.IsError {
		style = r.styles.Error
	}

	var b strings.Builder
	for i, l := range display {
		b.WriteString(style.Render("  " + l))
		if i < len(display)-1 || truncated > 0 {
			b.WriteString("\n")
		}
	}
	if truncated > 0 {
		b.WriteString(style.Render(fmt.Sprintf("  [%d more lines]", truncated)))
	}
	return b.String()
}

// CommandEntry shows a shell command the model wants to push to the shell buffer.
type CommandEntry struct{ Content string }

func (e CommandEntry) render(r Renderer) string {
	return r.styles.TWSymbol.Render("◆ ") + "$ " + r.styles.Command.Render(e.Content)
}

// ErrorEntry shows an inline error message.
type ErrorEntry struct{ Content string }

func (e ErrorEntry) render(r Renderer) string {
	return r.styles.Error.Render("Error: " + e.Content)
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func renderMarkdown(content string, gr *glamour.TermRenderer) string {
	if gr == nil {
		return content
	}
	rendered, err := gr.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimRight(rendered, "\n")
}

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
	return r.styles.UserSymbol.Render("› ") + hangingIndent(e.Content, 2)
}

// AssistantEntry is text output from the model (direct or via the respond tool).
type AssistantEntry struct{ Content string }

func (e AssistantEntry) render(r Renderer) string {
	text := strings.TrimLeft(renderMarkdown(e.Content, r.glamour), "\n")
	return r.styles.TWSymbol.Render("◆ ") + hangingIndent(text, 2)
}

// Tool call/result visual conventions:
//
//	⏺ bash(ls -la)
//	  ⎿  total 24
//	     drwxr-xr-x 3 user staff
//	     …
//	     12 more lines
//
// The call lives at column 0 so it reads as flush with normal thread content;
// the result corner glyph sits at column 2 and continuation lines align under
// the corner's content (column 5) so the result nests visibly beneath.
const (
	toolCallPrefix       = ""
	toolResultPrefix     = "  "
	toolResultContPrefix = "     "
	maxDisplayLines      = 5
)

// ToolCallEntry shows a tool being invoked.
type ToolCallEntry struct {
	Name   string // e.g. "bash"
	Detail string // e.g. the command or path
}

func (e ToolCallEntry) render(r Renderer) string {
	icon := r.styles.TWSymbol.Render("⏺ ")
	name := strings.ToLower(e.Name)
	body := name
	if e.Detail != "" {
		body = name + r.styles.ToolCall.Render("("+e.Detail+")")
	}
	return toolCallPrefix + icon + body
}

// ToolResultEntry shows the output of a tool call.
type ToolResultEntry struct {
	Content string
	IsError bool
}

func (e ToolResultEntry) render(r Renderer) string {
	lines := strings.Split(strings.TrimRight(e.Content, "\n"), "\n")
	total := len(lines)
	display := lines
	truncated := 0
	if total > maxDisplayLines {
		display = lines[:maxDisplayLines]
		truncated = total - maxDisplayLines
	}
	if len(display) == 0 {
		return ""
	}

	contentStyle := r.styles.ToolOutput
	if e.IsError {
		contentStyle = r.styles.Error
	}
	marker := r.styles.ToolCall.Render("⎿  ")

	var b strings.Builder
	b.WriteString(toolResultPrefix + marker + contentStyle.Render(display[0]))
	for _, l := range display[1:] {
		b.WriteString("\n" + toolResultContPrefix + contentStyle.Render(l))
	}
	if truncated > 0 {
		b.WriteString("\n" + toolResultContPrefix +
			r.styles.ToolCall.Render(fmt.Sprintf("… %d more lines", truncated)))
	}
	return b.String()
}

// CommandEntry shows a shell command the model wants to push to the shell buffer.
type CommandEntry struct{ Content string }

func (e CommandEntry) render(r Renderer) string {
	return r.styles.TWSymbol.Render("◆ ") + "$ " + r.styles.Command.Render(hangingIndent(e.Content, 4))
}

// ErrorEntry shows an inline error message.
type ErrorEntry struct{ Content string }

func (e ErrorEntry) render(r Renderer) string {
	return r.styles.Error.Render("Error: " + hangingIndent(e.Content, 2))
}

// SystemEntry is an informational message from termwise itself
// (e.g. /help output).
type SystemEntry struct{ Content string }

func (e SystemEntry) render(r Renderer) string {
	return r.styles.ActionHints.Render(hangingIndent(e.Content, 2))
}

// hangingIndent prefixes every line after the first with `width` spaces, so
// multi-line entry content aligns with the first line's content (after the
// leading icon).
func hangingIndent(s string, width int) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	pad := strings.Repeat(" ", width)
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = pad + lines[i]
	}
	return strings.Join(lines, "\n")
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

package agentui

import (
	"fmt"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"

	"github.com/matteo-psnt/termwise/internal/theme"
)

// Renderer holds the config needed to render thread entries.
// Passed as a value to each entry's render method, keeping rendering stateless
// with all presentation config in one place.
type Renderer struct {
	styles  Styles
	glamour *glamour.TermRenderer
}

func newRenderer(hasDarkBg bool, palette theme.Palette) Renderer {
	cfg := styles.LightStyleConfig
	if hasDarkBg {
		cfg = styles.DarkStyleConfig
	}
	var zero uint
	cfg.Document.Margin = &zero

	// glamour's default style configs prefix H2-H6 with their literal markdown
	// ("## ", "### "), which renders as visible punctuation in the thread. The
	// system prompt used to work around this by telling the model to avoid
	// anything below H1; clearing the prefixes fixes it at the source. Weight
	// and colour still separate the levels.
	cfg.H2.Prefix = ""
	cfg.H3.Prefix = ""
	cfg.H4.Prefix = ""
	cfg.H5.Prefix = ""
	cfg.H6.Prefix = ""

	gr, _ := glamour.NewTermRenderer(glamour.WithStyles(cfg), glamour.WithWordWrap(0))
	return Renderer{
		styles:  newStyles(hasDarkBg, palette),
		glamour: gr,
	}
}

// RenderThread renders all thread entries into the scrollback.
//
// Every entry hangs off a fixed gutter, and each new user message opens a
// visibly separated turn, so a long session can be scanned turn-by-turn
// instead of line-by-line.
func (r Renderer) RenderThread(entries []ThreadEntry, width int) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			if _, startsTurn := e.(UserEntry); startsTurn {
				b.WriteString("\n\n" + r.turnSeparator(width) + "\n\n")
			} else {
				b.WriteString("\n")
			}
		}
		b.WriteString(e.render(r))
	}
	return b.String()
}

// turnSeparator is a short recessive rule marking the boundary between turns.
// It is deliberately narrower than the pane so it reads as a divider inside the
// conversation rather than another frame around it.
func (r Renderer) turnSeparator(width int) string {
	n := min(max(width-gutterWidth, 8), 28)
	return r.styles.Separator.Render(strings.Repeat("╌", n))
}

// ThreadEntry is any displayable item in the conversation thread.
type ThreadEntry interface {
	render(r Renderer) string
}

// UserEntry is a message typed by the user.
type UserEntry struct{ Content string }

func (e UserEntry) render(r Renderer) string {
	return r.styles.UserSymbol.Render("›  ") + hangingIndent(e.Content, gutterWidth)
}

// AssistantEntry is text output from the model (direct or via the respond tool).
type AssistantEntry struct{ Content string }

func (e AssistantEntry) render(r Renderer) string {
	text := strings.TrimLeft(renderMarkdown(e.Content, r.glamour), "\n")
	return r.styles.TWSymbol.Render("◆  ") + hangingIndent(text, gutterWidth)
}

// Tool call/result visual conventions:
//
//	⏺ bash(ls -la)
//	  ⎿  total 24
//	     drwxr-xr-x 3 user staff
//	     …
//	     12 more lines
//
// Tool activity is indented into the content column rather than sharing the
// gutter with user and assistant turns, so the answer stays the thing the eye
// lands on and the work behind it reads as subordinate to it.
const (
	// gutterWidth is the fixed left column holding the ›, ◆ and ⏺ sigils.
	gutterWidth = 3

	toolCallPrefix       = "   "
	toolResultPrefix     = "     "
	toolResultContPrefix = "       "
	maxDisplayLines      = 5
)

// ToolCallEntry shows a tool being invoked.
type ToolCallEntry struct {
	Name   string // e.g. "bash"
	Detail string // e.g. the command or path
}

func (e ToolCallEntry) render(r Renderer) string {
	icon := r.styles.ToolCall.Render("⏺ ")
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
	marker := r.styles.ToolCall.Render("⎿ ")

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
	return r.styles.TWSymbol.Render("◆  ") + r.styles.ActionHints.Render("$ ") +
		r.styles.Command.Render(hangingIndent(e.Content, gutterWidth+2))
}

// ErrorEntry shows an inline error message.
type ErrorEntry struct{ Content string }

func (e ErrorEntry) render(r Renderer) string {
	return r.styles.Error.Render("!  " + hangingIndent(e.Content, gutterWidth))
}

// SystemEntry is an informational message from termwise itself
// (e.g. /help output).
type SystemEntry struct{ Content string }

func (e SystemEntry) render(r Renderer) string {
	return r.styles.ActionHints.Render("   " + hangingIndent(e.Content, gutterWidth))
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

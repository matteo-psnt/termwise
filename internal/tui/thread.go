package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// EntryKind identifies the type of a thread entry.
type EntryKind int

const (
	EntryUser      EntryKind = iota // "You: ..."
	EntryAssistant                  // bare text from model (implicit respond)
	EntryToolCall                   // "[bash] cmd" or "[read] path"
	EntryToolResult                 // "→ output"
	EntryRespond                    // final respond tool output
	EntryError                      // error inline
)

// ThreadEntry is one display item in the conversation thread.
type ThreadEntry struct {
	Kind        EntryKind
	Content     string
	ToolName    string // for EntryToolCall
	ToolDetail  string // for EntryToolCall: command or path
	Auto        bool   // for EntryToolCall: was auto-accepted
	RespondType string // for EntryRespond: "command" | "text"
	IsError     bool   // for EntryToolResult: marks as error result
}

const maxDisplayLines = 20

// renderThread renders all thread entries to a string for the viewport.
func renderThread(entries []ThreadEntry, width int, s Styles) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(renderEntry(e, width, s))
	}
	return b.String()
}

func renderEntry(e ThreadEntry, width int, s Styles) string {
	switch e.Kind {
	case EntryUser:
		return s.UserPrefix.Render("You: ") + e.Content

	case EntryAssistant:
		text := renderMarkdown(e.Content)
		return s.TWPrefix.Render("tw: ") + text

	case EntryToolCall:
		tag := s.ToolName.Render(fmt.Sprintf("[%s] %s", e.ToolName, e.ToolDetail))
		if !e.Auto {
			return tag
		}
		// Right-align "(auto)" marker.
		autoStr := s.ToolAuto.Render("(auto)")
		tagWidth := lipgloss.Width(tag)
		autoWidth := lipgloss.Width(autoStr)
		padding := width - tagWidth - autoWidth - 4
		if padding < 1 {
			padding = 1
		}
		return tag + strings.Repeat(" ", padding) + autoStr

	case EntryToolResult:
		return renderToolOutput(e.Content, e.IsError, s)

	case EntryRespond:
		if e.RespondType == "command" {
			return "  $ " + s.Command.Render(e.Content)
		}
		text := renderMarkdown(e.Content)
		return s.TWPrefix.Render("tw: ") + text

	case EntryError:
		return s.Error.Render("Error: " + e.Content)
	}
	return ""
}

// renderToolOutput formats tool result lines with "→" prefix, truncating to maxDisplayLines.
func renderToolOutput(content string, isErr bool, s Styles) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	total := len(lines)

	display := lines
	truncated := 0
	if total > maxDisplayLines {
		display = lines[:maxDisplayLines]
		truncated = total - maxDisplayLines
	}

	var b strings.Builder
	for i, l := range display {
		b.WriteString(s.ToolOutput.Render("→ " + l))
		if i < len(display)-1 || truncated > 0 {
			b.WriteString("\n")
		}
	}
	if truncated > 0 {
		b.WriteString(s.ToolOutput.Render(fmt.Sprintf("[output truncated — %d more lines]", truncated)))
	}
	return b.String()
}

func renderMarkdown(content string) string {
	rendered, err := glamour.Render(content, "auto")
	if err != nil {
		return content
	}
	return strings.TrimRight(rendered, "\n")
}

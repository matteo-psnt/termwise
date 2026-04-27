package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
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
func renderThread(entries []ThreadEntry, s Styles, glamour string) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(renderEntry(e, s, glamour))
	}
	return b.String()
}

func renderEntry(e ThreadEntry, s Styles, glamour string) string {
	switch e.Kind {
	case EntryUser:
		return s.UserSymbol.Render("› ") + e.Content

	case EntryAssistant:
		text := strings.TrimLeft(renderMarkdown(e.Content, glamour), "\n")
		return s.TWSymbol.Render("◆ ") + text

	case EntryToolCall:
		name := capitalizeFirst(e.ToolName)
		return s.ToolCall.Render(name + "(" + e.ToolDetail + ")")

	case EntryToolResult:
		return renderToolOutput(e.Content, e.IsError, s)

	case EntryRespond:
		if e.RespondType == "command" {
			return s.TWSymbol.Render("◆ ") + "$ " + s.Command.Render(e.Content)
		}
		text := strings.TrimLeft(renderMarkdown(e.Content, glamour), "\n")
		return s.TWSymbol.Render("◆ ") + text

	case EntryError:
		return s.Error.Render("Error: " + e.Content)
	}
	return ""
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// renderToolOutput formats tool result lines as an indented │ block, truncating to maxDisplayLines.
func renderToolOutput(content string, isErr bool, s Styles) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	total := len(lines)

	display := lines
	truncated := 0
	if total > maxDisplayLines {
		display = lines[:maxDisplayLines]
		truncated = total - maxDisplayLines
	}

	borderStyle := s.ToolOutput
	if isErr {
		borderStyle = s.Error
	}

	var b strings.Builder
	for i, l := range display {
		b.WriteString(borderStyle.Render("  "+l))
		if i < len(display)-1 || truncated > 0 {
			b.WriteString("\n")
		}
	}
	if truncated > 0 {
		b.WriteString(borderStyle.Render(fmt.Sprintf("  [%d more lines]", truncated)))
	}
	return b.String()
}

func renderMarkdown(content string, style string) string {
	rendered, err := glamour.Render(content, style)
	if err != nil {
		return content
	}
	return strings.TrimRight(rendered, "\n")
}

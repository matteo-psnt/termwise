package agentui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// slashPicker is a small list-picker used by slash commands like /effort,
// /theme, and /model. It renders in the input row when stateSlashPicker is active.
type slashPicker struct {
	cmd     string // command name including the leading slash, e.g. "/effort"
	title   string
	options []string
	cursor  int
}

func newSlashPicker(cmd, title string, options []string, current string) slashPicker {
	cursor := 0
	for i, opt := range options {
		if opt == current {
			cursor = i
			break
		}
	}
	return slashPicker{
		cmd:     cmd,
		title:   title,
		options: options,
		cursor:  cursor,
	}
}

// Update returns (updated, choice, submitted, cancelled).
func (p slashPicker) Update(msg tea.KeyMsg) (slashPicker, string, bool, bool) {
	switch msg.Type {
	case tea.KeyUp:
		if p.cursor > 0 {
			p.cursor--
		}
	case tea.KeyDown:
		if p.cursor < len(p.options)-1 {
			p.cursor++
		}
	case tea.KeyEnter:
		if len(p.options) == 0 {
			return p, "", false, true
		}
		return p, p.options[p.cursor], true, false
	case tea.KeyEsc:
		return p, "", false, true
	}
	switch msg.String() {
	case "ctrl+p":
		if p.cursor > 0 {
			p.cursor--
		}
	case "ctrl+n":
		if p.cursor < len(p.options)-1 {
			p.cursor++
		}
	case "k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "j":
		if p.cursor < len(p.options)-1 {
			p.cursor++
		}
	}
	return p, "", false, false
}

// View renders the picker in the input row.
func (p slashPicker) View(r Renderer) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", r.styles.InputPrompt.Render(" "+p.title))
	for i, opt := range p.options {
		cursor := "  "
		if i == p.cursor {
			cursor = "› "
			b.WriteString(r.styles.InputPrompt.Render(cursor+opt) + "\n")
		} else {
			b.WriteString(cursor + opt + "\n")
		}
	}
	b.WriteString(r.styles.ActionHints.Render("  [↑/↓] move   [↵] select   [esc] cancel"))
	return b.String()
}

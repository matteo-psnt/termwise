package agentui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// slashOption is one row in a slashPicker. Label is the primary text shown
// to the user; Detail is an optional dimmed secondary column (e.g. provider
// name); Value is the canonical string returned when the row is selected and
// passed to the command's Run handler.
type slashOption struct {
	Label  string
	Detail string
	Value  string
}

// simpleOptions builds slashOptions where Label == Value and Detail is empty.
// Used by /effort and /theme where no human name lookup is needed.
func simpleOptions(values []string) []slashOption {
	out := make([]slashOption, len(values))
	for i, v := range values {
		out[i] = slashOption{Label: v, Value: v}
	}
	return out
}

// slashPicker is a small list-picker used by slash commands like /effort,
// /theme, and /model. It renders in the input row when stateSlashPicker is active.
type slashPicker struct {
	cmd      string // command name including the leading slash, e.g. "/effort"
	title    string
	subtitle string // one-line dimmed description shown under the title
	options  []slashOption
	cursor   int
	current  string // the option Value matching today's setting (gets a ✔)
}

func newSlashPicker(cmd, title, subtitle string, options []slashOption, current string) slashPicker {
	cursor := 0
	for i, opt := range options {
		if opt.Value == current {
			cursor = i
			break
		}
	}
	return slashPicker{
		cmd:      cmd,
		title:    title,
		subtitle: subtitle,
		options:  options,
		cursor:   cursor,
		current:  current,
	}
}

// Update returns (updated, chosenValue, submitted, cancelled).
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
		return p, p.options[p.cursor].Value, true, false
	case tea.KeyEsc:
		return p, "", false, true
	}
	switch msg.String() {
	case "ctrl+p", "k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "ctrl+n", "j":
		if p.cursor < len(p.options)-1 {
			p.cursor++
		}
	}
	return p, "", false, false
}

// View renders the picker in the input row.
func (p slashPicker) View(r Renderer) string {
	var b strings.Builder
	b.WriteString(r.styles.InputPrompt.Render(" "+p.title) + "\n")
	if p.subtitle != "" {
		b.WriteString(r.styles.ActionHints.Render("  "+p.subtitle) + "\n")
	}
	b.WriteString("\n")

	// Label width includes the trailing check glyph so the Detail column aligns
	// whether or not a row has the ✔ marker.
	const checkSuffix = "  ✔"
	labelW := 0
	for _, opt := range p.options {
		w := lipgloss.Width(opt.Label) + lipgloss.Width(checkSuffix)
		if w > labelW {
			labelW = w
		}
	}

	for i, opt := range p.options {
		cursor := "  "
		if i == p.cursor {
			cursor = "❯ "
		}

		label := opt.Label
		if opt.Value == p.current {
			label += checkSuffix
		}
		labelPad := labelW - lipgloss.Width(label)
		if labelPad < 0 {
			labelPad = 0
		}

		if i == p.cursor {
			// Focused row: render the whole row in the highlight style so it reads
			// as one selection block — keeps the eye on a single moving target.
			line := cursor + label + strings.Repeat(" ", labelPad)
			if opt.Detail != "" {
				line += "   " + opt.Detail
			}
			b.WriteString(r.styles.InputPrompt.Render(line) + "\n")
		} else {
			line := cursor + label + strings.Repeat(" ", labelPad)
			if opt.Detail != "" {
				line += "   " + r.styles.ActionHints.Render(opt.Detail)
			}
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(r.styles.ActionHints.Render("  ↑/↓ navigate · ↵ select · esc cancel"))
	return b.String()
}

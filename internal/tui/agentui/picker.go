package agentui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// picker is the sub-model for the ask tool UI.
// It renders a selectable list of options with an "Other" free-text field.
type picker struct {
	question    string
	options     []string
	multiSelect bool

	cursor    int
	selected  map[int]bool
	otherText string
}

func newPicker(question string, options []string, multiSelect bool) picker {
	return picker{
		question:    question,
		options:     options,
		multiSelect: multiSelect,
		selected:    make(map[int]bool),
	}
}

// totalItems is the number of items including the "Other" row.
func (p picker) totalItems() int { return len(p.options) + 1 }

// onOther reports whether the cursor is on the "Other" row.
func (p picker) onOther() bool { return p.cursor == len(p.options) }

// Update handles keyboard input for the picker.
// Returns (updatedPicker, result, submitted, cancelled).
func (p picker) Update(msg tea.KeyPressMsg) (picker, string, bool, bool) {
	switch msg.Code {
	case tea.KeyUp:
		if p.cursor > 0 {
			p.cursor--
		}

	case tea.KeyDown:
		if p.cursor < p.totalItems()-1 {
			p.cursor++
		}

	case tea.KeySpace:
		// On the "Other" row space is ordinary text, not a toggle — free-text
		// answers are usually more than one word. Without this the key is
		// swallowed here and never reaches the printable-input default below.
		if p.onOther() {
			p.otherText += " "
		} else if p.multiSelect {
			p.selected[p.cursor] = !p.selected[p.cursor]
		}

	case tea.KeyEnter:
		if p.onOther() {
			if p.otherText != "" {
				return p, p.otherText, true, false
			}
			// Enter on empty "Other" — do nothing
		} else if !p.multiSelect {
			return p, p.options[p.cursor], true, false
		} else {
			// Multi-select: collect selected options.
			var results []string
			for i, opt := range p.options {
				if p.selected[i] {
					results = append(results, opt)
				}
			}
			if len(results) == 0 {
				// Nothing toggled — use cursor item.
				results = []string{p.options[p.cursor]}
			}
			return p, strings.Join(results, ", "), true, false
		}

	case tea.KeyEscape:
		return p, "User cancelled", false, true

	case tea.KeyBackspace:
		// Trim a rune, not a byte — byte slicing corrupts multi-byte input.
		if p.onOther() && p.otherText != "" {
			r := []rune(p.otherText)
			p.otherText = string(r[:len(r)-1])
		}

	default:
		// Printable input while editing the "Other" field.
		if p.onOther() && msg.Text != "" {
			p.otherText += msg.Text
		}
	}

	return p, "", false, false
}

// View renders the picker list.
func (p picker) View(_ Renderer) string {
	var b strings.Builder

	for i, opt := range p.options {
		cursor := "  "
		if p.cursor == i {
			cursor = "> "
		}
		if p.multiSelect {
			check := "[ ]"
			if p.selected[i] {
				check = "[x]"
			}
			fmt.Fprintf(&b, "%s%s %s\n", cursor, check, opt)
		} else {
			fmt.Fprintf(&b, "%s%s\n", cursor, opt)
		}
	}

	// "Other" row.
	otherCursor := "  "
	if p.onOther() {
		otherCursor = "> "
	}
	otherLine := fmt.Sprintf("%sOther: %s", otherCursor, p.otherText)
	if p.onOther() {
		otherLine += "█"
	}
	b.WriteString(otherLine)

	if p.multiSelect {
		b.WriteString("\n\n  space: toggle   enter: confirm")
	}

	return b.String()
}

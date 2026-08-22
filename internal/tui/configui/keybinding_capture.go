package configui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/matteo-psnt/termwise/internal/keybinding"
)

type keybindingModel struct {
	styles     configStyles
	current    string // displayed before capture
	captured   string
	confirming bool

	result    string
	done      bool
	cancelled bool
}

func newKeybindingCapture(current string, styles configStyles) keybindingModel {
	return keybindingModel{styles: styles, current: current}
}

func (keybindingModel) Init() tea.Cmd { return nil }

func (m keybindingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.confirming {
		switch key.String() {
		case "esc":
			m.cancelled = true
			m.done = true
		case "enter":
			m.result = m.captured
			m.done = true
		default:
			if binding, ok := keybinding.KeyMsgToZsh(key); ok {
				m.captured = binding
			}
		}
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.cancelled = true
		m.done = true
	default:
		if binding, ok := keybinding.KeyMsgToZsh(key); ok {
			m.captured = binding
			m.confirming = true
		}
	}
	return m, nil
}

func (m keybindingModel) View() string {
	var b strings.Builder
	b.WriteString("Shell keybinding:\n\n")
	if m.confirming {
		b.WriteString("Captured: " + m.styles.Selected.Render(keybinding.Label(m.captured)) + "\n\n")
		b.WriteString(m.styles.Hint.Render(`takes effect in new terminals, or run: eval "$(termwise init zsh)"`))
		b.WriteString("\n\n" + m.styles.Dim.Render("enter confirm   any key retry   esc cancel"))
	} else {
		b.WriteString("Current: " + m.styles.Normal.Render(keybinding.Label(m.current)) + "\n\n")
		b.WriteString("Press a key combination...\n")
		b.WriteString("\n" + m.styles.Dim.Render("esc cancel"))
	}
	return b.String()
}

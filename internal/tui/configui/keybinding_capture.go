package configui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/keybinding"
)

type keybindingModel struct {
	styles     configStyles
	current    string // displayed before capture
	captured   string
	confirming bool
	refused    string // why the last key pressed can't be used; shown until the next key

	result    string
	done      bool
	cancelled bool
}

func newKeybindingCapture(current string, styles configStyles) keybindingModel {
	return keybindingModel{styles: styles, current: current}
}

func (keybindingModel) Init() tea.Cmd { return nil }

func (m keybindingModel) Update(msg tea.Msg) (keybindingModel, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	m.refused = ""
	switch key.String() {
	case "esc":
		m.cancelled = true
		m.done = true
		return m, nil
	case "enter":
		if m.confirming {
			m.result = m.captured
			m.done = true
			return m, nil
		}
	}
	binding, ok := keybinding.KeyMsgToZsh(key.Key())
	if !ok {
		return m, nil
	}
	// A reserved key would take over basic shell editing in every terminal
	// once installed, so it is refused here rather than saved. Back in
	// capture mode, enter can't confirm the key captured before it.
	if reason, reserved := keybinding.Reserved(binding); reserved {
		m.refused = keybinding.Label(binding) + " can't be used: " + reason
		m.captured = ""
		m.confirming = false
		return m, nil
	}
	m.captured = binding
	m.confirming = true
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
		if m.refused != "" {
			b.WriteString("\n" + m.styles.Error.Render(m.refused) + "\n")
		}
		b.WriteString("\n" + m.styles.Dim.Render("esc cancel"))
	}
	return b.String()
}

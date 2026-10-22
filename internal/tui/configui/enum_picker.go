package configui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type enumPickerModel struct {
	styles  configStyles
	label   string
	options []string
	cursor  int

	result    string
	done      bool
	cancelled bool
}

func newEnumPicker(label, current string, options []string, styles configStyles) enumPickerModel {
	cursor := 0
	for i, opt := range options {
		if opt == current {
			cursor = i
			break
		}
	}
	return enumPickerModel{
		styles:  styles,
		label:   label,
		options: options,
		cursor:  cursor,
	}
}

func (enumPickerModel) Init() tea.Cmd { return nil }

func (m enumPickerModel) Update(msg tea.Msg) (enumPickerModel, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if c, ok := moveListCursor(key.String(), m.cursor, len(m.options)); ok {
		m.cursor = c
		return m, nil
	}
	switch key.String() {
	case "esc", "b":
		m.cancelled = true
		m.done = true
	case "enter", " ":
		m.result = m.options[m.cursor]
		m.done = true
	}
	return m, nil
}

func (m enumPickerModel) View() string {
	var b strings.Builder
	b.WriteString(m.label + ":\n\n")
	for i, opt := range m.options {
		b.WriteString(renderListRow(m.styles, i == m.cursor, opt, "") + "\n")
	}
	b.WriteString("\n" + m.styles.Dim.Render("↑/↓ navigate   enter select   esc cancel"))
	return b.String()
}

package agentui

import (
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/theme"
)

// pickerModel is an idle Model with just enough wiring for the picker flows:
// a renderer and input so theme previews can restyle them, and a viewport so
// refreshViewport has somewhere to write.
func pickerModel(themeName string) Model {
	return Model{
		sharedState: sharedState{
			input:     textarea.New(),
			renderer:  newRenderer(true, theme.Get(themeName), 0),
			hasDarkBg: true,
			themeName: themeName,
			histIdx:   -1,
			vp:        viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
		},
		state: stateIdle,
	}
}

func pressKey(t *testing.T, m Model, msg tea.KeyPressMsg) Model {
	t.Helper()
	next, _ := modeFor(m.state).handleKey(m, msg)
	return next.(Model)
}

var (
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
	keyUp    = tea.KeyPressMsg{Code: tea.KeyUp}
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc   = tea.KeyPressMsg{Code: tea.KeyEscape}
)

func openThemePicker(t *testing.T) (Model, []string) {
	t.Helper()
	names := theme.Names()
	if len(names) < 2 {
		t.Fatalf("need at least two themes to move the cursor, have %v", names)
	}
	next, _ := runTheme(pickerModel(names[0]), nil)
	m := next.(Model)
	if m.state != stateSlashPicker || m.slashPicker == nil {
		t.Fatalf("/theme did not open the picker: state %v", m.state)
	}
	return m, names
}

// Moving the cursor previews the highlighted value live; Esc puts the value
// the picker opened with back.
func TestSlashPickerPreviewsOnMoveAndRevertsOnCancel(t *testing.T) {
	m, names := openThemePicker(t)

	m = pressKey(t, m, keyDown)
	if m.themeName != names[1] {
		t.Fatalf("after down, theme = %q, want preview of %q", m.themeName, names[1])
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	if m.themeName != names[0] {
		t.Fatalf("after k, theme = %q, want preview of %q", m.themeName, names[0])
	}
	m = pressKey(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.themeName != names[1] {
		t.Fatalf("after j, theme = %q, want preview of %q", m.themeName, names[1])
	}

	m = pressKey(t, m, keyEsc)
	if m.themeName != names[0] {
		t.Fatalf("after esc, theme = %q, want original %q restored", m.themeName, names[0])
	}
	if m.state != stateIdle || m.slashPicker != nil {
		t.Fatalf("esc left state %v with picker %v, want idle and closed", m.state, m.slashPicker)
	}
	if len(m.thread) != 0 {
		t.Fatalf("cancel wrote to the thread: %#v", m.thread)
	}
}

// A key that does not move the cursor must not re-preview: Up at the top of
// the list leaves everything as it was.
func TestSlashPickerIgnoresKeysThatDoNotMoveTheCursor(t *testing.T) {
	m, names := openThemePicker(t)

	m = pressKey(t, m, keyUp)
	if m.slashPicker == nil || m.slashPicker.cursor != 0 {
		t.Fatalf("up at the top moved the cursor: %+v", m.slashPicker)
	}
	if m.themeName != names[0] {
		t.Fatalf("theme changed to %q without the cursor moving", m.themeName)
	}
}

// Enter runs the command with the highlighted value and closes the picker.
func TestSlashPickerCommitRunsCommand(t *testing.T) {
	m, names := openThemePicker(t)

	m = pressKey(t, m, keyDown)
	m = pressKey(t, m, keyEnter)
	if m.themeName != names[1] {
		t.Fatalf("after enter, theme = %q, want %q", m.themeName, names[1])
	}
	if m.state != stateIdle || m.slashPicker != nil {
		t.Fatalf("enter left state %v with picker %v, want idle and closed", m.state, m.slashPicker)
	}
	want := "Theme set to " + names[1] + "."
	if len(m.thread) != 1 {
		t.Fatalf("thread = %#v, want one confirmation entry", m.thread)
	}
	if e, ok := m.thread[0].(SystemEntry); !ok || e.Content != want {
		t.Fatalf("thread[0] = %#v, want SystemEntry %q", m.thread[0], want)
	}
}

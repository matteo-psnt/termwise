package configui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestEditorListScrollsToCursor(t *testing.T) {
	const height = 12 // the list alone is 15 lines
	m, _ := update(t, thinkingEditor(""), tea.WindowSizeMsg{Width: 100, Height: height})
	for i := 0; i < len(m.rows); i++ {
		m = press(t, m, keyDown)
	}
	last := m.rows[m.cursor]
	if last.kind != rowSetting {
		t.Fatalf("cursor on %+v, want the last setting", last)
	}

	box := m.styles.Outer.Render(m.renderInner())
	if h := lipgloss.Height(box); h > height {
		t.Errorf("box is %d lines on a %d-line screen", h, height)
	}
	if !strings.Contains(box, "▶") || !strings.Contains(box, "↑ more") {
		t.Errorf("focused row or scroll marker missing:\n%s", box)
	}
	if strings.Contains(box, "Providers") {
		t.Errorf("top of the list still shown:\n%s", box)
	}

	// Back at the top the first rows show again.
	for i := 0; i < len(m.rows); i++ {
		m = press(t, m, keyUp)
	}
	box = m.styles.Outer.Render(m.renderInner())
	if !strings.Contains(box, "Providers") || !strings.Contains(box, "↓ more") {
		t.Errorf("top of the list not shown:\n%s", box)
	}
}

func TestEditorFitsNarrowTerminal(t *testing.T) {
	const width = 60
	m := thinkingEditor("")
	pc := m.cfg.Providers["anthropic"]
	pc.Model = "claude-sonnet-4-6-with-a-very-long-fine-tune-suffix-20991231"
	m.cfg.Providers["anthropic"] = pc
	m, _ = update(t, m, tea.WindowSizeMsg{Width: width, Height: 60})
	m = openRow(t, m, settingRow("shell_context")) // toggles; cursor stays on the long description
	box := m.styles.Outer.Render(m.renderInner())
	if w := lipgloss.Width(box); w > width {
		t.Errorf("box is %d columns on a %d-column screen:\n%s", w, width, box)
	}
	if !strings.Contains(box, "secrets.") {
		t.Errorf("description cut off instead of wrapped:\n%s", box)
	}
}

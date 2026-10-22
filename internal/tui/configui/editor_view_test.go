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

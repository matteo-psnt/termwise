package configui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func pressKB(m keybindingModel, keys ...tea.KeyPressMsg) keybindingModel {
	for _, k := range keys {
		m, _ = m.Update(k)
	}
	return m
}

func TestKeybindingCaptureRefusesReservedKeys(t *testing.T) {
	ctrlC := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	m := pressKB(newKeybindingCapture("^T", newStylesForTheme(true, "")), ctrlC)
	if m.confirming || m.done || m.captured != "" {
		t.Fatalf("confirming = %v, done = %v, captured = %q; want Ctrl+C refused", m.confirming, m.done, m.captured)
	}
	if view := m.View(); !strings.Contains(view, "Ctrl+C can't be used") {
		t.Errorf("no reason shown:\n%s", view)
	}

	// Refused while confirming: back to capture, so enter can't save the
	// key captured before it.
	m = pressKB(m, tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl}, tea.KeyPressMsg{Code: 'i', Mod: tea.ModCtrl}, keyEnter)
	if m.done || m.confirming {
		t.Fatalf("done = %v, confirming = %v; want Ctrl+I refused and capture kept open", m.done, m.confirming)
	}

	// The message clears and a free key is still taken.
	m = pressKB(m, tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	if !m.confirming || m.captured != "^G" || strings.Contains(m.View(), "can't be used") {
		t.Errorf("confirming = %v, captured = %q; want ^G captured and the message gone:\n%s", m.confirming, m.captured, m.View())
	}
}

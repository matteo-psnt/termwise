package agentui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func key(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

// typeInto drives the picker's "Other" field with a literal string.
func typeInto(p picker, s string) picker {
	for _, r := range s {
		if r == ' ' {
			p, _, _, _ = p.Update(key(tea.KeySpace, " "))
			continue
		}
		p, _, _, _ = p.Update(key(r, string(r)))
	}
	return p
}

// Space on the "Other" row used to hit the multi-select toggle case and get
// swallowed, so free-text answers could never contain a space.
func TestPickerOtherAcceptsSpaces(t *testing.T) {
	for _, multi := range []bool{false, true} {
		p := newPicker("q", []string{"a", "b"}, multi)
		p.cursor = len(p.options) // move to "Other"
		p = typeInto(p, "the gemini cli tool")
		if p.otherText != "the gemini cli tool" {
			t.Errorf("multiSelect=%v: otherText = %q, want %q", multi, p.otherText, "the gemini cli tool")
		}
	}
}

// Space must still toggle when the cursor is on a real option.
func TestPickerSpaceStillTogglesOptions(t *testing.T) {
	p := newPicker("q", []string{"a", "b"}, true)
	p, _, _, _ = p.Update(key(tea.KeySpace, " "))
	if !p.selected[0] {
		t.Fatal("space did not toggle option 0")
	}
	p, _, _, _ = p.Update(key(tea.KeySpace, " "))
	if p.selected[0] {
		t.Fatal("second space did not untoggle option 0")
	}
}

func TestPickerBackspaceIsRuneAware(t *testing.T) {
	p := newPicker("q", []string{"a"}, false)
	p.cursor = len(p.options)
	p = typeInto(p, "café")
	p, _, _, _ = p.Update(key(tea.KeyBackspace, ""))
	if p.otherText != "caf" {
		t.Errorf("otherText = %q, want %q", p.otherText, "caf")
	}
	// Backspace on empty must not panic.
	for range 5 {
		p, _, _, _ = p.Update(key(tea.KeyBackspace, ""))
	}
}

func TestPickerOtherSubmits(t *testing.T) {
	p := newPicker("q", []string{"a"}, false)
	p.cursor = len(p.options)
	p = typeInto(p, "brew install gemini-cli")
	_, result, submitted, cancelled := p.Update(key(tea.KeyEnter, ""))
	if !submitted || cancelled {
		t.Fatalf("submitted=%v cancelled=%v, want true/false", submitted, cancelled)
	}
	if result != "brew install gemini-cli" {
		t.Errorf("result = %q", result)
	}
}

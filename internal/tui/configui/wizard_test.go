package configui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestWizardCtrlCQuitsWhileFetching(t *testing.T) {
	m := newWizardModel(true, "", nil)
	m.provider = "openai"
	m.step = wizWorking

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !next.(wizardModel).done || cmd == nil {
		t.Fatal("ctrl+c ignored while fetching models")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c did not quit")
	}
}

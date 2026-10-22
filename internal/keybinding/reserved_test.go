package keybinding

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestReserved(t *testing.T) {
	for _, k := range []tea.Key{
		{Code: 'm', Mod: tea.ModCtrl},
		{Code: 'j', Mod: tea.ModCtrl},
		{Code: 'i', Mod: tea.ModCtrl},
		{Code: 'h', Mod: tea.ModCtrl},
		{Code: '?', Mod: tea.ModCtrl},
		{Code: 'c', Mod: tea.ModCtrl},
		{Code: 'd', Mod: tea.ModCtrl},
		{Code: 'z', Mod: tea.ModCtrl},
		{Code: 's', Mod: tea.ModCtrl},
		{Code: 'q', Mod: tea.ModCtrl},
		{Code: '[', Mod: tea.ModAlt},
		{Code: 'O', Mod: tea.ModAlt},
	} {
		zsh, ok := KeyMsgToZsh(k)
		if !ok {
			t.Errorf("KeyMsgToZsh(%v) not capturable", k)
			continue
		}
		if reason, reserved := Reserved(zsh); !reserved || reason == "" {
			t.Errorf("Reserved(%q) = %q,%v; want a reason", zsh, reason, reserved)
		}
	}
	if _, reserved := Reserved("^["); !reserved {
		t.Error("Esc not reserved")
	}
	// Ordinary shortcuts, including Alt and Ctrl+Alt forms of reserved keys,
	// stay free.
	for _, s := range []string{"^T", "^G", "^@", `\ef`, `\eo`, `\e^M`, `\e[15~`, `\e[A`} {
		if reason, reserved := Reserved(s); reserved {
			t.Errorf("Reserved(%q) = %q; want it free", s, reason)
		}
	}
}

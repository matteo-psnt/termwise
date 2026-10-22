package agentui

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/keybinding"
	"github.com/matteo-psnt/termwise/internal/theme"
)

// configEditorModel returns a Model with /config open on an empty on-disk
// config, so commits can be checked against the file.
func configEditorModel(t *testing.T) Model {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := config.SaveConfig(path, config.FileConfig{}); err != nil {
		t.Fatal(err)
	}
	m := pickerModel(theme.DefaultName)
	m.cfgPath = path
	m.suggestions = true
	return m.openConfigEditor()
}

func loadConfig(t *testing.T, path string) config.FileConfig {
	t.Helper()
	cfg, exists, err := config.LoadConfig(path)
	if err != nil || !exists {
		t.Fatalf("loading config: exists=%v err=%v", exists, err)
	}
	return cfg
}

func mustSetting(t *testing.T, key string) config.SettingDef {
	t.Helper()
	def, ok := config.FindSetting(key)
	if !ok {
		t.Fatalf("no %q setting", key)
	}
	return def
}

// chooserDetail returns the Detail column the chooser shows for key.
func chooserDetail(t *testing.T, m Model, key string) string {
	t.Helper()
	for _, opt := range m.configEditor.chooser.options {
		if opt.Value == key {
			return opt.Detail
		}
	}
	t.Fatalf("chooser has no %q row", key)
	return ""
}

func TestConfigEditorValuePreviewsAndRevertsOnCancel(t *testing.T) {
	m := configEditorModel(t)
	m = openConfigValuePicker(m, mustSetting(t, "suggestions"))

	m = pressKey(t, m, keyDown) // On → Off
	if m.suggestions {
		t.Fatal("moving to Off did not preview suggestions off")
	}

	m = pressKey(t, m, keyEsc)
	if !m.suggestions {
		t.Fatal("cancel did not restore suggestions on")
	}
	if m.configEditor == nil || m.configEditor.phase != configPhaseChooser {
		t.Fatalf("cancel did not return to the chooser: %+v", m.configEditor)
	}
	if got := loadConfig(t, m.cfgPath).Settings.Suggestions; got != nil {
		t.Fatalf("cancel wrote suggestions=%v to disk", *got)
	}
}

func TestConfigEditorValueCommitPersistsAndRefreshesChooser(t *testing.T) {
	m := configEditorModel(t)
	m = openConfigValuePicker(m, mustSetting(t, "suggestions"))

	m = pressKey(t, m, keyDown)
	m = pressKey(t, m, keyEnter)

	if m.suggestions {
		t.Fatal("commit did not apply suggestions off")
	}
	if got := loadConfig(t, m.cfgPath).Settings.Suggestions; got == nil || *got {
		t.Fatalf("commit wrote suggestions=%v, want false", got)
	}
	if m.configEditor.phase != configPhaseChooser || m.configEditor.valuePicker != nil {
		t.Fatalf("commit did not return to the chooser: %+v", m.configEditor)
	}
	if d := chooserDetail(t, m, "suggestions"); d != "Off" {
		t.Fatalf("chooser shows %q for suggestions, want Off", d)
	}
	if m.state != stateConfigEditor {
		t.Fatalf("commit left state %v, want the editor still open", m.state)
	}
}

func TestConfigEditorCaptureCommitsKeybinding(t *testing.T) {
	m := configEditorModel(t)
	m = openConfigValuePicker(m, mustSetting(t, "keybinding"))
	if m.configEditor.phase != configPhaseCapture {
		t.Fatalf("keybinding opened phase %v, want capture", m.configEditor.phase)
	}

	key := tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl}
	want, ok := keybinding.KeyMsgToZsh(key.Key())
	if !ok {
		t.Fatal("ctrl+x has no zsh form")
	}
	m = pressKey(t, m, key)

	if m.closeKey != want {
		t.Fatalf("closeKey = %q, want %q", m.closeKey, want)
	}
	if got := loadConfig(t, m.cfgPath).Settings.Keybinding; got != want {
		t.Fatalf("disk keybinding = %q, want %q", got, want)
	}
	if d := chooserDetail(t, m, "keybinding"); d != want {
		t.Fatalf("chooser shows %q for keybinding, want %q", d, want)
	}
	if m.configEditor.phase != configPhaseChooser {
		t.Fatalf("capture did not return to the chooser: phase %v", m.configEditor.phase)
	}
}

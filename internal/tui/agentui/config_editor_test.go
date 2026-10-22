package agentui

import (
	"os"
	"path/filepath"
	"strings"
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

// An env-only setup has a config file with no [providers] table. Writing the
// effort or the active model must create the entry, not panic on a nil map.
func TestConfigWritesSurviveMissingProvidersTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := config.SaveConfig(path, config.FileConfig{}); err != nil {
		t.Fatal(err)
	}
	m := pickerModel(theme.DefaultName)
	m.cfgPath = path
	m.providerName = "anthropic"
	m.modelID = "claude-sonnet-4-6"

	next, _ := runEffort(m, []string{"high"})
	m = next.(Model)
	if got := loadConfig(t, path).Providers["anthropic"].Effort; got != "high" {
		t.Fatalf("effort on disk = %q, want high", got)
	}

	if err := config.SaveConfig(path, config.FileConfig{}); err != nil {
		t.Fatal(err)
	}
	if err := m.persistActiveModel(); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(t, path)
	if cfg.Providers["anthropic"].Model != "claude-sonnet-4-6" || cfg.SelectedProvider != "anthropic" {
		t.Fatalf("persisted %+v selected %q", cfg.Providers, cfg.SelectedProvider)
	}
}

// In keybinding capture the current close key is recorded, not obeyed.
func TestKeybindingCaptureRecordsTheCloseKey(t *testing.T) {
	m := configEditorModel(t)
	m.closeKey = "^T"
	m = openConfigValuePicker(m, mustSetting(t, "keybinding"))

	next, _ := m.handleKey(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	got := next.(Model)
	if got.quitting {
		t.Fatal("pressing the close key during capture quit the TUI")
	}
	if v := loadConfig(t, got.cfgPath).Settings.Keybinding; v != "^T" {
		t.Fatalf("keybinding on disk = %q, want ^T", v)
	}
}

// Keys the shell needs for basic editing are refused, with a reason, and the
// capture stays open for another key, as in `termwise config`.
func TestConfigEditorCaptureRefusesReservedKeys(t *testing.T) {
	m := configEditorModel(t)
	before := loadConfig(t, m.cfgPath).Settings.Keybinding
	m = openConfigValuePicker(m, mustSetting(t, "keybinding"))

	m = pressKey(t, m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if m.configEditor.phase != configPhaseCapture {
		t.Fatalf("Ctrl+D ended the capture: phase %v", m.configEditor.phase)
	}
	if got := loadConfig(t, m.cfgPath).Settings.Keybinding; got != before {
		t.Fatalf("Ctrl+D was saved as the keybinding: %q", got)
	}
	if !strings.Contains(m.configEditor.captureNotice, "Ctrl+D") {
		t.Errorf("notice %q does not say why Ctrl+D was refused", m.configEditor.captureNotice)
	}
	if !strings.Contains(renderCaptureView(m), m.configEditor.captureNotice) {
		t.Error("the refusal is not shown in the capture view")
	}
}

// A setting that can't be written must say so, not report success while the
// file stays unchanged.
func TestSettingSaveFailureIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	m := configEditorModel(t)
	dir := filepath.Dir(m.cfgPath)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	def := mustSetting(t, "theme")
	if err := m.persistConfigValue(def, "dracula"); err == nil {
		t.Fatal("saving into a read-only directory reported success")
	}
}

// Without a config file (env-key-only setup) a setting applies for the
// session but no file is created: one holding only that setting would have
// no selected provider and break every later start.
func TestSettingWithoutConfigFileIsNotWritten(t *testing.T) {
	m := configEditorModel(t)
	if err := os.Remove(m.cfgPath); err != nil {
		t.Fatal(err)
	}
	if err := m.persistConfigValue(mustSetting(t, "theme"), "dracula"); err != nil {
		t.Fatalf("no-file save returned %v, want nil", err)
	}
	if config.ConfigExists(m.cfgPath) {
		t.Error("a config file was created for a single setting")
	}
}

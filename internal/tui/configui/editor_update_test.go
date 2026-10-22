package configui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
)

var (
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyUp    = tea.KeyPressMsg{Code: tea.KeyUp}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
)

func press(t *testing.T, m editorModel, keys ...tea.KeyPressMsg) editorModel {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(k)
		m = next.(editorModel)
	}
	return m
}

// openRow puts the cursor on the first row matching match and presses enter.
func openRow(t *testing.T, m editorModel, match func(editorRow) bool) editorModel {
	t.Helper()
	for i, row := range m.rows {
		if match(row) {
			m.cursor = i
			return press(t, m, keyEnter)
		}
	}
	t.Fatal("no matching row")
	return m
}

func settingRow(key string) func(editorRow) bool {
	return func(r editorRow) bool { return r.kind == rowSetting && config.Settings[r.settingIdx].Key == key }
}

func effortRow(r editorRow) bool { return r.kind == rowProvEffort }

func thinkingEditor(effort string) editorModel {
	cfg := config.FileConfig{
		SelectedProvider: "anthropic",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {Auth: "env", Model: "claude-sonnet-4-6", Effort: effort},
		},
	}
	return newEditorModel("", cfg, true)
}

func TestEffortPickerCommitsToProvider(t *testing.T) {
	// Levels are low, medium, high; the picker opens on the current one.
	m := openRow(t, thinkingEditor(""), effortRow)
	if m.enumPicker == nil {
		t.Fatal("effort row did not open the enum picker")
	}
	m = press(t, m, keyDown, keyEnter)
	if m.enumPicker != nil {
		t.Fatal("picker still open after enter")
	}
	if got := m.cfg.Providers["anthropic"].Effort; got != "high" {
		t.Errorf("Effort = %q, want high", got)
	}
	if got := m.cfg.Settings; got != (config.SettingsConfig{}) {
		t.Errorf("effort pick touched settings: %+v", got)
	}
}

func TestEffortPickerOmitsDefault(t *testing.T) {
	m := openRow(t, thinkingEditor("high"), effortRow)
	m = press(t, m, keyUp, keyEnter)
	if got := m.cfg.Providers["anthropic"].Effort; got != "" {
		t.Errorf("Effort = %q, want empty for the default", got)
	}
}

func TestEffortPickerCancelKeepsValue(t *testing.T) {
	m := openRow(t, thinkingEditor("low"), effortRow)
	m = press(t, m, keyDown, keyEsc)
	if m.enumPicker != nil {
		t.Fatal("picker still open after esc")
	}
	if got := m.cfg.Providers["anthropic"].Effort; got != "low" {
		t.Errorf("Effort = %q, want low", got)
	}
}

func TestKeybindingCaptureCommitsToSetting(t *testing.T) {
	m := openRow(t, thinkingEditor(""), settingRow("keybinding"))
	if m.keybinding == nil {
		t.Fatal("keybinding row did not open the capture")
	}
	m = press(t, m, tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl}, keyEnter)
	if got := m.cfg.Settings.Keybinding; got != "^G" {
		t.Errorf("Keybinding = %q, want ^G", got)
	}
}

func TestThemePickerCommitsToSetting(t *testing.T) {
	m := openRow(t, thinkingEditor(""), settingRow("theme"))
	if m.themePicker == nil {
		t.Fatal("theme row did not open the picker")
	}
	want := m.themePicker.PreviewName()
	m = press(t, m, keyEnter)
	if got := m.cfg.Settings.Theme; got != want {
		t.Errorf("Theme = %q, want %q", got, want)
	}
}

func modelRow(r editorRow) bool { return r.kind == rowProvModel }

func update(t *testing.T, m editorModel, msg tea.Msg) (editorModel, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(editorModel), cmd
}

func TestEditorTakesConnResultWithSubModelOpen(t *testing.T) {
	m := openRow(t, thinkingEditor(""), modelRow)
	if m.modelPicker == nil {
		t.Fatal("model row did not open the picker")
	}
	m, _ = update(t, m, editorConnMsg{provider: "anthropic", seq: m.connSeq})
	if !m.connChecked || !m.connOK {
		t.Errorf("connChecked = %v, connOK = %v; want the result recorded", m.connChecked, m.connOK)
	}
}

func TestEditorSpinnerTicksWithSubModelOpen(t *testing.T) {
	m := openRow(t, thinkingEditor(""), modelRow)
	before := m.spin.View()
	m, cmd := update(t, m, m.spin.Tick())
	if cmd == nil || m.spin.View() == before {
		t.Error("editor spinner did not advance while the picker was open")
	}
}

func TestEditorResizeWithSubModelOpen(t *testing.T) {
	m := openRow(t, thinkingEditor(""), modelRow)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 120 || m.height != 40 {
		t.Errorf("editor size = %dx%d, want 120x40", m.width, m.height)
	}
	if m.modelPicker.height != 40 {
		t.Errorf("picker height = %d, want 40", m.modelPicker.height)
	}
}

func TestAddWizardOpensWithEditorSize(t *testing.T) {
	m, _ := update(t, thinkingEditor(""), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = openRow(t, m, func(r editorRow) bool { return r.kind == rowAddProvider })
	if m.addWizard == nil {
		t.Fatal("add row did not open the wizard")
	}
	if m.addWizard.height != 30 {
		t.Errorf("wizard height = %d, want 30", m.addWizard.height)
	}
}

func TestEditorDropsStaleConnResult(t *testing.T) {
	m := thinkingEditor("")
	first := m.connSeq
	m = press(t, m, keyEsc) // revert starts a new check
	m, _ = update(t, m, editorConnMsg{provider: "anthropic", seq: first})
	if m.connChecked {
		t.Error("accepted the result of a superseded check")
	}
	m, _ = update(t, m, editorConnMsg{provider: "openai", seq: m.connSeq})
	if m.connChecked {
		t.Error("accepted a result for a provider that isn't active")
	}
	m, _ = update(t, m, editorConnMsg{provider: "anthropic", seq: m.connSeq})
	if !m.connChecked || !m.connOK {
		t.Error("dropped the current result")
	}
}

func TestModelPickerDropsCancelledPickersResult(t *testing.T) {
	cfg := config.FileConfig{
		SelectedProvider: "anthropic",
		Providers: map[string]config.ProviderConfig{
			"anthropic": {Model: "claude-sonnet-4-6"},
			"openai":    {Model: "gpt-5"},
		},
	}
	m := newEditorModel("", cfg, true)
	m = openRow(t, m, func(r editorRow) bool { return r.kind == rowProvModel && r.provider == "anthropic" })
	stale := editorModelsMsg{provider: "anthropic", seq: m.modelPicker.seq, models: []provider.Model{{ID: "claude-x"}}}
	m = press(t, m, keyEsc)

	m = openRow(t, m, func(r editorRow) bool { return r.kind == rowProvModel && r.provider == "openai" })
	m, _ = update(t, m, stale)
	if !m.modelPicker.loading || len(m.modelPicker.models) != 0 {
		t.Errorf("openai picker took anthropic's late list: %+v", m.modelPicker.models)
	}

	// Same provider reopened: still a different request.
	m = press(t, m, keyEsc)
	m = openRow(t, m, func(r editorRow) bool { return r.kind == rowProvModel && r.provider == "anthropic" })
	m, _ = update(t, m, stale)
	if !m.modelPicker.loading {
		t.Error("reopened picker took the cancelled picker's list")
	}

	m, _ = update(t, m, editorModelsMsg{provider: "anthropic", seq: m.modelPicker.seq, models: []provider.Model{{ID: "claude-y"}}})
	if m.modelPicker.loading || len(m.modelPicker.models) != 1 {
		t.Error("picker dropped its own result")
	}
}

// The editor box must fit the screen at any cursor position, even on short,
// narrow terminals where long descriptions wrap to many lines.
func TestEditorFitsShortNarrowTerminals(t *testing.T) {
	for _, size := range []struct{ w, h int }{{40, 12}, {20, 15}, {60, 10}, {80, 24}} {
		m, _ := update(t, thinkingEditor(""), tea.WindowSizeMsg{Width: size.w, Height: size.h})
		for i := range m.rows {
			m.cursor = i
			if h := lipgloss.Height(m.styles.Outer.Render(m.renderInner())); h > size.h {
				t.Errorf("%dx%d, row %d: box is %d lines tall", size.w, size.h, i, h)
				break
			}
		}
	}
}

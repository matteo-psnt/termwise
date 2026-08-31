package agentui

import (
	"strings"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/theme"
)

// configEditorPhase indicates which view of the /config editor is currently
// active. The editor is a two-level UI: a chooser that lists settings, then a
// per-setting value picker once a row is selected.
type configEditorPhase int

const (
	configPhaseChooser configEditorPhase = iota
	configPhaseValue                     // a value picker is open (toggle/theme/enum)
	configPhaseCapture                   // a keybinding capture is open
)

// configEditor owns the state of the /config editor while it is open. It
// snapshots the on-disk config at open time so the chooser can display current
// values, and tracks the active sub-mode for value editing.
type configEditor struct {
	cfg     config.FileConfig
	chooser slashPicker
	phase   configEditorPhase

	// Set while phase != configPhaseChooser. setting holds the SettingDef being
	// edited; valuePicker is the active per-Kind picker (nil for capture);
	// original is the at-open-of-value-picker value used to revert on Esc.
	setting     config.SettingDef
	valuePicker *slashPicker
	original    string
}

// configChooserOptions builds the chooser rows from the canonical config.Settings
// registry. Each row uses the setting's resolved value as Detail so the user
// sees the current state at a glance.
func configChooserOptions(cfg config.FileConfig) []slashOption {
	out := make([]slashOption, 0, len(config.Settings))
	for _, s := range config.Settings {
		out = append(out, slashOption{
			Label:  s.Label,
			Detail: s.Resolve(cfg),
			Value:  s.Key,
		})
	}
	return out
}

// valuePickerOptions returns the option list shown in the value picker for
// the given setting Kind. Returns nil for kinds that don't use a picker
// (currently just KindKeybinding, which uses a capture sub-mode).
func valuePickerOptions(def config.SettingDef) []slashOption {
	switch def.Kind {
	case config.KindToggle:
		return simpleOptions([]string{"On", "Off"})
	case config.KindTheme:
		return simpleOptions(theme.Names())
	case config.KindEnum:
		return simpleOptions(def.Options)
	}
	return nil
}

// applyConfigValue applies the in-memory effect of a setting value. This is
// what the value picker calls on every cursor move (live preview) and on
// cancel (revert). Settings without an in-TUI side effect (auto_resume,
// keybinding stored only on disk) are no-ops here; persistence is separate.
func applyConfigValue(m Model, def config.SettingDef, val string) Model {
	switch def.Key {
	case "theme":
		if name := theme.Normalize(val); theme.IsValid(name) {
			m.applyTheme(name)
		}
	case "llm_judge":
		m.llmJudge = strings.EqualFold(val, "on")
	case "suggestions":
		m.suggestions = strings.EqualFold(val, "on")
	case "keybinding":
		m.closeKey = val
	}
	return m
}

// persistConfigValue writes a setting value to disk using the canonical
// SettingDef.Set hook. Reads the latest on-disk config first so we don't clobber
// concurrent edits from outside the TUI.
func (m Model) persistConfigValue(def config.SettingDef, val string) {
	if m.cfgPath == "" {
		return
	}
	cfg, exists, err := config.LoadConfig(m.cfgPath)
	if err != nil || !exists {
		return
	}
	def.Set(&cfg, val)
	_ = config.SaveConfig(m.cfgPath, cfg)
}

// openConfigEditor loads the current config snapshot, builds the chooser, and
// transitions the model into stateConfigEditor.
func (m Model) openConfigEditor() Model {
	var cfg config.FileConfig
	if m.cfgPath != "" {
		loaded, exists, err := config.LoadConfig(m.cfgPath)
		if err == nil && exists {
			cfg = loaded
		}
	}
	chooser := newSlashPicker("/config", "Settings", "Choose a setting to edit.", configChooserOptions(cfg), "")
	m.configEditor = &configEditor{
		cfg:     cfg,
		chooser: chooser,
		phase:   configPhaseChooser,
	}
	m.state = stateConfigEditor
	return m
}

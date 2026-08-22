package configui

import (
	"sort"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/keybinding"
	"github.com/matteo-psnt/termwise/internal/theme"
)

// ---------------------------------------------------------------------------
// Row types
// ---------------------------------------------------------------------------

type editorRowKind int

const (
	rowSectionHeader editorRowKind = iota
	rowProvHeader
	rowProvModel
	rowProvAuth
	rowAddProvider
	rowSetting // driven by editorSettings table; settingIdx identifies which one
)

type editorRow struct {
	kind       editorRowKind
	provider   string
	label      string
	settingIdx int // only meaningful when kind == rowSetting
}

// hintForKind returns the keyboard hint line for a setting row.
func hintForKind(kind config.SettingKind) string {
	switch kind {
	case config.KindToggle:
		return "enter toggle   ↑/↓ navigate   q quit"
	case config.KindEnum, config.KindTheme:
		return "enter pick   ↑/↓ navigate   q quit"
	case config.KindKeybinding:
		return "enter edit   ↑/↓ navigate   q quit"
	default:
		return "enter edit   ↑/↓ navigate   q quit"
	}
}

// displayValue formats a setting's value for display in the TUI list.
// Kind-specific formatting (e.g. keybinding label) is applied here so the
// registry stays free of TUI-layer concerns.
func displayValue(def config.SettingDef, cfg config.FileConfig) string {
	val := def.Get(cfg)
	switch def.Kind {
	case config.KindToggle:
		return def.Resolve(cfg)
	case config.KindKeybinding:
		return keybinding.Label(def.Resolve(cfg))
	case config.KindTheme:
		return theme.Label(val)
	default:
		if val == "" {
			resolved := def.Resolve(cfg)
			if resolved != "" && resolved != val {
				return resolved + " (default)"
			}
		}
		return val
	}
}

// activateSetting handles Enter on a setting row: toggles booleans inline,
// opens the appropriate sub-model for all other kinds.
func activateSetting(m editorModel, settingIdx int) editorModel {
	def := config.Settings[settingIdx]
	switch def.Kind {
	case config.KindToggle:
		current, _ := def.GetAny(m.cfg).(bool)
		if current {
			def.Set(&m.cfg, "Off")
		} else {
			def.Set(&m.cfg, "On")
		}
	case config.KindTheme:
		tp := newThemePicker(def.Get(m.cfg), m.r, m.styles)
		m.themePicker = &tp
		m.activeSettingIdx = settingIdx
	case config.KindKeybinding:
		kb := def.Resolve(m.cfg)
		kbm := newKeybindingCapture(kb, m.styles)
		m.keybinding = &kbm
		m.activeSettingIdx = settingIdx
	case config.KindEnum:
		ep := newEnumPicker(def.Label, def.Resolve(m.cfg), def.Options, m.styles)
		m.enumPicker = &ep
		m.activeSettingIdx = settingIdx
	}
	return m
}

// ---------------------------------------------------------------------------
// Row list management
// ---------------------------------------------------------------------------

func (m *editorModel) buildRows() {
	m.rows = []editorRow{{kind: rowSectionHeader, label: "Providers"}}
	for _, name := range m.sortedProviders() {
		m.rows = append(m.rows,
			editorRow{kind: rowProvHeader, provider: name},
			editorRow{kind: rowProvModel, provider: name},
			editorRow{kind: rowProvAuth, provider: name},
		)
	}
	if len(m.cfg.Providers) < len(config.ProviderInfos()) {
		m.rows = append(m.rows, editorRow{kind: rowAddProvider})
	}
	m.rows = append(m.rows, editorRow{kind: rowSectionHeader, label: "Settings"})
	for i := range config.Settings {
		m.rows = append(m.rows, editorRow{kind: rowSetting, settingIdx: i})
	}
	m.snapCursor()
}

func (m *editorModel) snapCursor() {
	for m.cursor < len(m.rows) && m.rows[m.cursor].kind == rowSectionHeader {
		m.cursor++
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
}

func (m editorModel) moveCursor(delta int) editorModel {
	next := m.cursor + delta
	for next >= 0 && next < len(m.rows) {
		if m.rows[next].kind != rowSectionHeader {
			m.cursor = next
			return m
		}
		next += delta
	}
	return m
}

func (m editorModel) sortedProviders() []string {
	names := make([]string, 0, len(m.cfg.Providers))
	for name := range m.cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func describeAuth(providerName string, pc config.ProviderConfig) string {
	if providerName == "ollama" {
		if pc.BaseURL == "" {
			return "base URL · (default)"
		}
		return "base URL · " + pc.BaseURL
	}
	method := pc.Auth
	if method == "" {
		method = "env"
	}
	switch method {
	case "env":
		v := pc.EnvVar
		if v == "" {
			v = "(default)"
		}
		return "env · " + v
	case "cmd":
		v := pc.APIKeyCmd
		if v == "" {
			v = "(not set)"
		}
		if len(v) > 40 {
			v = v[:37] + "..."
		}
		return "cmd · " + v
	case "keychain":
		v := pc.KeychainEntry
		if v == "" {
			v = "(not set)"
		}
		return "keychain · " + v
	default:
		return method
	}
}

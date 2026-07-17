package config

import (
	"fmt"
	"strings"

	"github.com/matteo-psnt/termwise/internal/theme"
)

// SettingKind identifies how a setting is displayed and edited.
type SettingKind int

const (
	KindToggle     SettingKind = iota // boolean On/Off
	KindEnum                          // one of a fixed set of strings (future)
	KindKeybinding                    // captured interactively from a keypress
	KindTheme                         // selected from the theme picker
)

// SettingDef is a complete, self-contained description of one configurable setting.
// It is the single source of truth for display, editing, validation, show output,
// and JSON output. All surfaces that touch a setting are derived from this definition.
//
// To add a new boolean setting:
//
//  1. Add the field to SettingsConfig in schema.go.
//  2. Add one boolSetting(...) entry to Settings below.
//     Nothing else changes — validation, display, TUI editing, and show output are automatic.
type SettingDef struct {
	Key     string // TOML key name (e.g. "auto_resume"), used in show output
	Label   string // human-readable label (e.g. "Auto resume")
	Group   string // optional group label — reserved for future sectioning
	Kind    SettingKind
	Options []string // valid values for KindEnum; auto-used in Validate

	// Get returns the raw stored value as a string (empty if not set).
	// Used for display and as the input to Validate.
	Get func(cfg FileConfig) string

	// Resolve returns the effective value with defaults applied — never empty
	// for settings that have a default. Use this where the raw empty value
	// would be incorrect (e.g. building a runtime config).
	Resolve func(cfg FileConfig) string

	// Set applies a string value to the config pointer.
	// The value is expected to be in the same format that Get returns.
	Set func(cfg *FileConfig, val string)

	// GetAny returns the typed value for JSON output (bool, string, etc.).
	GetAny func(cfg FileConfig) any

	// Validate checks the raw value from Get for correctness.
	// Returns a non-nil error with a human-readable message when invalid.
	// Nil means no validation is needed (e.g. booleans, free-form strings).
	Validate func(val string) error
}

// Settings is the canonical registry of all user-configurable settings.
// Validation, the TUI editor, tw config show, and JSON output are all derived
// from this slice. No other code should hard-code setting-specific logic.
var Settings = []SettingDef{
	themeSetting(),
	keybindingSetting(),
	boolSetting("llm_judge", "LLM judge", false,
		func(cfg FileConfig) *bool { return cfg.Settings.LLMJudge },
		func(cfg *FileConfig, v *bool) { cfg.Settings.LLMJudge = v },
	),
	boolSetting("auto_resume", "Auto resume", false,
		func(cfg FileConfig) *bool { return cfg.Settings.AutoResume },
		func(cfg *FileConfig, v *bool) { cfg.Settings.AutoResume = v },
	),
	boolSetting("suggestions", "Suggestions", true,
		func(cfg FileConfig) *bool { return cfg.Settings.Suggestions },
		func(cfg *FileConfig, v *bool) { cfg.Settings.Suggestions = v },
	),
	effortSetting(),
}

// themeSetting builds the theme SettingDef, including validation and default resolution.
func themeSetting() SettingDef {
	get := func(cfg FileConfig) string { return cfg.Settings.Theme }
	return SettingDef{
		Key:   "theme",
		Label: "Theme",
		Kind:  KindTheme,
		Get:   get,
		Resolve: func(cfg FileConfig) string {
			return theme.Normalize(get(cfg)) // Normalize returns theme.DefaultName when empty
		},
		Set:    func(cfg *FileConfig, v string) { cfg.Settings.Theme = theme.Normalize(v) },
		GetAny: func(cfg FileConfig) any { return get(cfg) },
		Validate: func(val string) error {
			if val == "" {
				return nil // empty = use default; valid
			}
			if !theme.IsValid(val) {
				return fmt.Errorf(
					"unknown settings.theme %q\n  Valid themes: %s",
					val, strings.Join(theme.Names(), ", "),
				)
			}
			return nil
		},
	}
}

// keybindingSetting builds the keybinding SettingDef.
func keybindingSetting() SettingDef {
	const defaultKB = "^T"
	get := func(cfg FileConfig) string { return cfg.Settings.Keybinding }
	return SettingDef{
		Key:   "keybinding",
		Label: "Keybinding",
		Kind:  KindKeybinding,
		Get:   get,
		Resolve: func(cfg FileConfig) string {
			if v := get(cfg); v != "" {
				return v
			}
			return defaultKB
		},
		Set:    func(cfg *FileConfig, v string) { cfg.Settings.Keybinding = v },
		GetAny: func(cfg FileConfig) any { return get(cfg) },
		// No Validate: any keystring is accepted.
	}
}

// boolSetting constructs a SettingDef for a *bool field.
// defaultOn is the effective value when the field has never been set (nil).
// Get preserves the raw stored state by returning empty for nil.
func boolSetting(key, label string, defaultOn bool, get func(FileConfig) *bool, set func(*FileConfig, *bool)) SettingDef {
	resolve := func(v *bool) bool {
		if v == nil {
			return defaultOn
		}
		return *v
	}
	formatBool := func(v bool) string {
		if v {
			return "On"
		}
		return "Off"
	}
	getString := func(cfg FileConfig) string {
		raw := get(cfg)
		if raw == nil {
			return ""
		}
		return formatBool(*raw)
	}
	return SettingDef{
		Key:     key,
		Label:   label,
		Kind:    KindToggle,
		Get:     getString,
		Resolve: func(cfg FileConfig) string { return formatBool(resolve(get(cfg))) },
		Set: func(cfg *FileConfig, val string) {
			on := strings.EqualFold(val, "on") || strings.EqualFold(val, "true")
			set(cfg, &on)
		},
		GetAny: func(cfg FileConfig) any { return resolve(get(cfg)) },
	}
}

// EffortLevels lists valid reasoning effort values.
var EffortLevels = []string{"low", "medium", "high"}

// DefaultEffort is the effective effort when none is configured.
const DefaultEffort = "medium"

// effortSetting builds the effort SettingDef.
func effortSetting() SettingDef {
	get := func(cfg FileConfig) string { return cfg.Settings.Effort }
	return SettingDef{
		Key:     "effort",
		Label:   "Reasoning effort",
		Kind:    KindEnum,
		Options: EffortLevels,
		Get:     get,
		Resolve: func(cfg FileConfig) string {
			if v := get(cfg); v != "" {
				return v
			}
			return DefaultEffort
		},
		Set:    func(cfg *FileConfig, v string) { cfg.Settings.Effort = v },
		GetAny: func(cfg FileConfig) any { return get(cfg) },
		Validate: func(val string) error {
			if val == "" {
				return nil
			}
			for _, l := range EffortLevels {
				if val == l {
					return nil
				}
			}
			return fmt.Errorf("unknown effort %q — valid values: %s", val, strings.Join(EffortLevels, ", "))
		},
	}
}

// ResolveBoolSetting returns the effective boolean value for a setting by key,
// applying its declared default when the field has never been explicitly set.
func ResolveBoolSetting(cfg FileConfig, key string) bool {
	if s, ok := FindSetting(key); ok {
		if v, ok := s.GetAny(cfg).(bool); ok {
			return v
		}
	}
	return false
}

// FindSetting returns the SettingDef with the given key, or (SettingDef{}, false).
func FindSetting(key string) (SettingDef, bool) {
	for _, s := range Settings {
		if s.Key == key {
			return s, true
		}
	}
	return SettingDef{}, false
}

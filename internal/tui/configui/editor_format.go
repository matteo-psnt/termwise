package configui

import (
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/keybinding"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/theme"
)

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

// displayEffort formats a provider's stored effort value for display.
// Empty falls back to "<default> (default)" so the effective value is always visible.
func displayEffort(stored string) string {
	if stored == "" {
		return config.DefaultEffort + " (default)"
	}
	return stored
}

// describeAuth formats a provider's auth settings for its Auth row.
func describeAuth(providerName string, pc config.ProviderConfig) string {
	if provider.HasNoAuth(providerName) {
		if pc.BaseURL == "" {
			return "base URL · (default)"
		}
		return "base URL · " + pc.BaseURL
	}
	switch method := pc.AuthMethod(); method {
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

// settingLabelWidth returns the column width to pad setting labels to,
// computed from the longest registered setting label plus one space.
func settingLabelWidth() int {
	const minWidth = 13
	w := minWidth
	for _, s := range config.Settings {
		if n := len(s.Label) + 1; n > w {
			w = n
		}
	}
	return w
}

// navTail is the part of every editor-row hint that doesn't depend on the row.
const navTail = "↑/↓ navigate   esc revert   q quit"

// hintForKind returns the keyboard hint line for a setting row.
func hintForKind(kind config.SettingKind) string {
	switch kind {
	case config.KindToggle:
		return "enter toggle   " + navTail
	case config.KindEnum, config.KindTheme:
		return "enter pick   " + navTail
	default:
		return "enter edit   " + navTail
	}
}

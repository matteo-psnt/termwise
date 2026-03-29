package config

import (
	"fmt"
	"strings"

	"github.com/matteo-psnt/termwise/internal/theme"
)

// GetValue returns the string value of a config key by dot-path.
// Supported paths: active_provider, shell.keybinding, tui.show_footer,
// tui.theme, providers.<name>.<field>.
func GetValue(cfg Config, key string) (string, error) {
	parts := strings.SplitN(key, ".", 3)
	switch parts[0] {
	case "active_provider":
		return cfg.ActiveProvider, nil

	case "shell":
		if len(parts) < 2 {
			return "", fmt.Errorf("incomplete key %q", key)
		}
		switch parts[1] {
		case "keybinding":
			return cfg.Shell.Keybinding, nil
		}

	case "tui":
		if len(parts) < 2 {
			return "", fmt.Errorf("incomplete key %q", key)
		}
		switch parts[1] {
		case "show_footer":
			if cfg.TUI.ShowFooter == nil {
				return "true", nil
			}
			if *cfg.TUI.ShowFooter {
				return "true", nil
			}
			return "false", nil
		case "theme":
			if cfg.TUI.Theme == "" {
				return theme.DefaultName, nil
			}
			return theme.Normalize(cfg.TUI.Theme), nil
		}

	case "providers":
		if len(parts) < 3 {
			return "", fmt.Errorf("incomplete key %q — expected providers.<name>.<field>", key)
		}
		name, field := parts[1], parts[2]
		pc, ok := cfg.Providers[name]
		if !ok {
			return "", fmt.Errorf("provider %q is not configured", name)
		}
		switch field {
		case "auth_method":
			return pc.AuthMethod, nil
		case "env_var":
			return pc.EnvVar, nil
		case "api_key_cmd":
			return pc.APIKeyCmd, nil
		case "keychain_entry":
			return pc.KeychainEntry, nil
		case "model":
			return pc.Model, nil
		case "base_url":
			return pc.BaseURL, nil
		}
	}

	return "", fmt.Errorf("unknown config key %q", key)
}

// SetValue sets a config value by dot-path key. Does not save to disk.
func SetValue(cfg *Config, key, value string) error {
	parts := strings.SplitN(key, ".", 3)
	switch parts[0] {
	case "active_provider":
		cfg.ActiveProvider = value
		return nil

	case "shell":
		if len(parts) < 2 {
			return fmt.Errorf("incomplete key %q", key)
		}
		switch parts[1] {
		case "keybinding":
			cfg.Shell.Keybinding = value
			return nil
		}

	case "tui":
		if len(parts) < 2 {
			return fmt.Errorf("incomplete key %q", key)
		}
		switch parts[1] {
		case "show_footer":
			b := value == "true" || value == "1" || value == "yes"
			cfg.TUI.ShowFooter = &b
			return nil
		case "theme":
			cfg.TUI.Theme = theme.Normalize(value)
			return nil
		}

	case "providers":
		if len(parts) < 3 {
			return fmt.Errorf("incomplete key %q — expected providers.<name>.<field>", key)
		}
		name, field := parts[1], parts[2]
		pc := cfg.Providers[name]
		switch field {
		case "auth_method":
			pc.AuthMethod = value
		case "env_var":
			pc.EnvVar = value
		case "api_key_cmd":
			pc.APIKeyCmd = value
		case "keychain_entry":
			pc.KeychainEntry = value
		case "model":
			pc.Model = value
		case "base_url":
			pc.BaseURL = value
		default:
			return fmt.Errorf("unknown provider field %q", field)
		}
		cfg.SetProvider(name, pc)
		return nil
	}

	return fmt.Errorf("unknown config key %q", key)
}

// UnsetValue removes a config value or entire block by dot-path key.
// Supports: providers.<name> (removes the whole provider block).
func UnsetValue(cfg *Config, key string) error {
	parts := strings.SplitN(key, ".", 2)
	if parts[0] == "providers" && len(parts) == 2 {
		name := parts[1]
		if cfg.ActiveProvider == name {
			return fmt.Errorf("cannot unset %q — it is the active provider. Change active_provider first.", name)
		}
		delete(cfg.Providers, name)
		return nil
	}
	return fmt.Errorf("unset only supports removing provider blocks (providers.<name>)")
}

// IsModelKey reports whether key is a provider model field.
// Used to trigger API validation on set.
func IsModelKey(key string) (providerName string, ok bool) {
	parts := strings.SplitN(key, ".", 3)
	if len(parts) == 3 && parts[0] == "providers" && parts[2] == "model" {
		return parts[1], true
	}
	return "", false
}

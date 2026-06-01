package config

import (
	"fmt"
	"strings"
)

// Validate checks a FileConfig for structural correctness without making any
// network calls. Returns a list of human-readable diagnostics; empty means valid.
func Validate(cfg FileConfig) []string {
	var errs []string

	// Validate all settings through the registry so each SettingDef owns its own rules.
	for _, def := range Settings {
		if def.Validate == nil {
			continue
		}
		if err := def.Validate(def.Get(cfg)); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if cfg.SelectedProvider == "" {
		errs = append(errs, "selected_provider is not set — run `tw config` to set up")
		return errs
	}

	if !isKnownProvider(cfg.SelectedProvider) {
		errs = append(errs, fmt.Sprintf(
			"unknown provider %q\n  Supported: %s",
			cfg.SelectedProvider, strings.Join(ProviderNames(), ", "),
		))
	}

	pc, ok := cfg.Providers[cfg.SelectedProvider]
	if !ok {
		errs = append(errs, fmt.Sprintf(
			"provider %q is selected but has no config block\n  Add a [providers.%s] block or run `tw config`",
			cfg.SelectedProvider, cfg.SelectedProvider,
		))
		return errs
	}

	errs = append(errs, validateProviderConfig(cfg.SelectedProvider, pc)...)
	return errs
}

// ValidateForRuntime joins validation errors into a single error suitable for
// display at startup. Returns nil when valid.
func ValidateForRuntime(cfg FileConfig) error {
	errs := Validate(cfg)
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(errs, "\n"))
}

func validateProviderConfig(name string, pc ProviderConfig) []string {
	var errs []string

	if pc.Model == "" {
		errs = append(errs, fmt.Sprintf(
			"missing model in [providers.%s]\n  Run: tw config model set <model>",
			name,
		))
	}

	method := pc.Auth
	if method == "" {
		method = "env"
	}
	switch method {
	case "env", "cmd", "keychain":
		// valid
	default:
		errs = append(errs, fmt.Sprintf(
			"unknown auth %q in [providers.%s]\n  Valid methods: env, cmd, keychain",
			method, name,
		))
	}

	return errs
}

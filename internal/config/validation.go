package config

import (
	"fmt"
	"strings"
)

// knownProviders lists all provider names termwise knows about.
var knownProviders = []string{
	"anthropic", "openai", "ollama", "groq", "deepseek", "mistral",
}

// Validate performs structural validation of the config.
// It does not make any network calls.
// Returns a list of human-readable error strings (empty = valid).
func Validate(cfg Config) []string {
	var errs []string

	if cfg.ActiveProvider == "" {
		errs = append(errs, "active_provider is not set — run `tw config` to set up")
		return errs // nothing else to check without a provider
	}

	if !isKnownProvider(cfg.ActiveProvider) {
		errs = append(errs, fmt.Sprintf(
			"unknown provider %q\n  Supported providers: %s",
			cfg.ActiveProvider, strings.Join(knownProviders, ", "),
		))
	}

	pc, ok := cfg.Providers[cfg.ActiveProvider]
	if !ok {
		errs = append(errs, fmt.Sprintf(
			"provider %q is set as active but has no config\n  Add a [providers.%s] block or run `tw config`",
			cfg.ActiveProvider, cfg.ActiveProvider,
		))
		return errs
	}

	errs = append(errs, validateProviderConfig(cfg.ActiveProvider, pc)...)
	return errs
}

func validateProviderConfig(name string, pc ProviderConfig) []string {
	var errs []string

	if pc.Model == "" {
		errs = append(errs, fmt.Sprintf(
			"missing 'model' in [providers.%s]\n  Run `tw config set providers.%s.model <model>`",
			name, name,
		))
	}

	method := pc.AuthMethod
	if method == "" {
		method = "env"
	}

	switch method {
	case "env", "cmd", "keychain":
		// valid
	default:
		errs = append(errs, fmt.Sprintf(
			"unknown auth_method %q in [providers.%s]\n  Valid methods: env, cmd, keychain",
			method, name,
		))
	}

	return errs
}

func isKnownProvider(name string) bool {
	for _, p := range knownProviders {
		if p == name {
			return true
		}
	}
	return false
}

// ValidateForRuntime is a convenience wrapper that returns a single combined error
// suitable for display at `tw "..."` startup. Returns nil if valid.
func ValidateForRuntime(cfg Config) error {
	errs := Validate(cfg)
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(errs, "\n"))
}

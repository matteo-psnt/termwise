package config

import (
	"fmt"
	"os"

	"github.com/matteo-psnt/termwise/internal/cliname"
	"github.com/matteo-psnt/termwise/internal/models"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// Environment variables that override the config file for one run, so a
// script or CI job can pick a provider and model without writing config.
const (
	EnvProvider = "TERMWISE_PROVIDER"
	EnvModel    = "TERMWISE_MODEL"
)

// applyEnvOverrides applies TERMWISE_PROVIDER and TERMWISE_MODEL to cfg in
// memory. A provider with no block in the file gets the same defaults
// `config use` would write.
func applyEnvOverrides(cfg *FileConfig) error {
	if name := os.Getenv(EnvProvider); name != "" {
		if err := UseProvider(cfg, name); err != nil {
			return fmt.Errorf("%s: %w", EnvProvider, err)
		}
	}
	if model := os.Getenv(EnvModel); model != "" && cfg.SelectedProvider != "" {
		pc := cfg.Providers[cfg.SelectedProvider]
		pc.Model = model
		SetProvider(cfg, cfg.SelectedProvider, pc)
	}
	return nil
}

// ZeroConfigDefaults builds a minimal FileConfig from well-known API key
// environment variables when no config file exists. It returns the first
// provider whose env var is set, or (FileConfig{}, false) if none match.
// Providers that need no key (Ollama, LM Studio) are skipped: there is no
// env var to detect them by.
func ZeroConfigDefaults() (FileConfig, bool) {
	for _, info := range provider.Catalog() {
		if info.DefaultEnvVar == "" || os.Getenv(info.DefaultEnvVar) == "" {
			continue
		}
		return FileConfig{
			SelectedProvider: info.Name,
			Providers: map[string]ProviderConfig{
				info.Name: {
					Auth:   "env",
					EnvVar: info.DefaultEnvVar,
					Model:  models.DefaultModel(info.Name),
				},
			},
		}, true
	}
	return FileConfig{}, false
}

// EnvOnlyNotice describes the zero-config fallback when it is in effect:
// there is no file at path, so the provider comes from an API key in the
// environment. It returns "" when a config file exists or no key is set.
func EnvOnlyNotice(path string) string {
	if ConfigExists(path) {
		return ""
	}
	cfg, ok := ZeroConfigDefaults()
	if !ok {
		return ""
	}
	name := cfg.SelectedProvider
	pc := cfg.Providers[name]
	return fmt.Sprintf("No config file — using %s via $%s (model %s). Run `%s config` to save or change this.",
		displayName(name), pc.EnvVar, pc.Model, cliname.Name())
}

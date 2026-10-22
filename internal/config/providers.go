package config

import "github.com/matteo-psnt/termwise/internal/provider"

// DefaultEnvVar returns the conventional API key env var for a provider.
// Returns "" for providers that need no key (e.g. Ollama) or for unknown names.
func DefaultEnvVar(name string) string {
	info, ok := provider.Lookup(name)
	if !ok {
		return ""
	}
	return info.DefaultEnvVar
}

// displayName returns a provider's human-readable name, or name itself for
// one that is not in the catalog.
func displayName(name string) string {
	if info, ok := provider.Lookup(name); ok {
		return info.DisplayName
	}
	return name
}

func isKnownProvider(name string) bool {
	_, ok := provider.Lookup(name)
	return ok
}

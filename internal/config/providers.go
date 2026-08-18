package config

import "github.com/matteo-psnt/termwise/internal/provider"

// ProviderInfo describes a built-in provider. Re-exports provider.Info so
// config-package callers don't need to import the provider package directly.
type ProviderInfo = provider.Info

// ProviderInfos returns the built-in provider catalog in display order.
func ProviderInfos() []ProviderInfo {
	return provider.Catalog()
}

// ProviderNames returns the name of every built-in provider in display order.
func ProviderNames() []string {
	return provider.Names()
}

// DefaultEnvVar returns the conventional API key env var for a provider.
// Returns "" for providers that need no key (e.g. Ollama) or for unknown names.
func DefaultEnvVar(name string) string {
	info, ok := provider.Lookup(name)
	if !ok {
		return ""
	}
	return info.DefaultEnvVar
}

func isKnownProvider(name string) bool {
	_, ok := provider.Lookup(name)
	return ok
}

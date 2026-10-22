package config

import (
	"strings"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// DefaultEnvVar returns the conventional API key env var for a provider.
// Returns "" for providers that need no key (e.g. Ollama) or for unknown names.
func DefaultEnvVar(name string) string {
	info, ok := provider.Lookup(name)
	if !ok {
		return ""
	}
	return info.DefaultEnvVar
}

// displayName returns a provider's human-readable name for use in a
// sentence, or name itself for one that is not in the catalog. The catalog
// marks self-hosted providers "Ollama (local)" so the setup list reads
// right, but "Can't reach Ollama (local) at …" does not, so the marker is
// dropped here.
func displayName(name string) string {
	if info, ok := provider.Lookup(name); ok {
		return strings.TrimSuffix(info.DisplayName, " (local)")
	}
	return name
}

func isKnownProvider(name string) bool {
	_, ok := provider.Lookup(name)
	return ok
}

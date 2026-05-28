package config

// ProviderInfo describes a built-in provider.
type ProviderInfo struct {
	Name          string
	Label         string
	DefaultEnvVar string
}

// providerCatalog is the authoritative list of built-in providers.
// Order determines display order in the UI and zero-config detection priority.
var providerCatalog = []ProviderInfo{
	{Name: "anthropic", Label: "Anthropic", DefaultEnvVar: "ANTHROPIC_API_KEY"},
	{Name: "openai", Label: "OpenAI", DefaultEnvVar: "OPENAI_API_KEY"},
	{Name: "groq", Label: "Groq", DefaultEnvVar: "GROQ_API_KEY"},
	{Name: "deepseek", Label: "DeepSeek", DefaultEnvVar: "DEEPSEEK_API_KEY"},
	{Name: "mistral", Label: "Mistral", DefaultEnvVar: "MISTRAL_API_KEY"},
	{Name: "ollama", Label: "Ollama (local)"},
}

// ProviderInfos returns a copy of the built-in provider list in display order.
func ProviderInfos() []ProviderInfo {
	out := make([]ProviderInfo, len(providerCatalog))
	copy(out, providerCatalog)
	return out
}

// ProviderNames returns the name of every built-in provider.
func ProviderNames() []string {
	names := make([]string, len(providerCatalog))
	for i, info := range providerCatalog {
		names[i] = info.Name
	}
	return names
}

// DefaultEnvVar returns the conventional API key env var for a provider.
// Returns "" for providers that need no key (e.g. Ollama).
func DefaultEnvVar(name string) string {
	info, ok := lookupProvider(name)
	if !ok {
		return ""
	}
	return info.DefaultEnvVar
}

// fallbackEnvVar returns the env var to try as a last-resort fallback when
// configured auth fails. Only set for providers that have a canonical env var.
func fallbackEnvVar(name string) string {
	return DefaultEnvVar(name)
}

func isKnownProvider(name string) bool {
	_, ok := lookupProvider(name)
	return ok
}

func lookupProvider(name string) (ProviderInfo, bool) {
	for _, info := range providerCatalog {
		if info.Name == name {
			return info, true
		}
	}
	return ProviderInfo{}, false
}

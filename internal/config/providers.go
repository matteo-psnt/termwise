package config

// ProviderInfo describes one built-in provider.
type ProviderInfo struct {
	Name           string
	Label          string
	DefaultEnvVar  string
	FallbackEnvVar string
}

var providerCatalog = []ProviderInfo{
	{
		Name:           "anthropic",
		Label:          "Anthropic",
		DefaultEnvVar:  "ANTHROPIC_API_KEY",
		FallbackEnvVar: "ANTHROPIC_API_KEY",
	},
	{
		Name:           "openai",
		Label:          "OpenAI",
		DefaultEnvVar:  "OPENAI_API_KEY",
		FallbackEnvVar: "OPENAI_API_KEY",
	},
	{
		Name:          "groq",
		Label:         "Groq",
		DefaultEnvVar: "GROQ_API_KEY",
	},
	{
		Name:          "deepseek",
		Label:         "DeepSeek",
		DefaultEnvVar: "DEEPSEEK_API_KEY",
	},
	{
		Name:          "mistral",
		Label:         "Mistral",
		DefaultEnvVar: "MISTRAL_API_KEY",
	},
	{
		Name:  "ollama",
		Label: "Ollama (local)",
	},
}

// ProviderInfos returns the built-in providers in UI/display order.
func ProviderInfos() []ProviderInfo {
	out := make([]ProviderInfo, len(providerCatalog))
	copy(out, providerCatalog)
	return out
}

// ProviderNames returns the built-in provider names.
func ProviderNames() []string {
	names := make([]string, 0, len(providerCatalog))
	for _, info := range providerCatalog {
		names = append(names, info.Name)
	}
	return names
}

// DefaultEnvVar returns the conventional env var name for a provider.
func DefaultEnvVar(provider string) string {
	info, ok := providerInfo(provider)
	if !ok {
		return ""
	}
	return info.DefaultEnvVar
}

func fallbackEnvVar(provider string) string {
	info, ok := providerInfo(provider)
	if !ok {
		return ""
	}
	return info.FallbackEnvVar
}

func isKnownProvider(name string) bool {
	_, ok := providerInfo(name)
	return ok
}

func providerInfo(name string) (ProviderInfo, bool) {
	for _, info := range providerCatalog {
		if info.Name == name {
			return info, true
		}
	}
	return ProviderInfo{}, false
}

package openaicompat

import "github.com/matteo-psnt/termwise/internal/provider"

// compatProvider describes one OpenAI-compatible endpoint. Slice order
// determines display order in the registry.
type compatProvider struct {
	name          string
	displayName   string
	defaultEnvVar string
	defaultURL    string
	noAuth        bool
}

var compatProviders = []compatProvider{
	{name: "openai", displayName: "OpenAI", defaultEnvVar: "OPENAI_API_KEY", defaultURL: "https://api.openai.com/v1"},
	{name: "groq", displayName: "Groq", defaultEnvVar: "GROQ_API_KEY", defaultURL: "https://api.groq.com/openai/v1"},
	{name: "deepseek", displayName: "DeepSeek", defaultEnvVar: "DEEPSEEK_API_KEY", defaultURL: "https://api.deepseek.com"},
	{name: "mistral", displayName: "Mistral", defaultEnvVar: "MISTRAL_API_KEY", defaultURL: "https://api.mistral.ai/v1"},
	{name: "ollama", displayName: "Ollama (local)", defaultEnvVar: "", defaultURL: "http://localhost:11434/v1", noAuth: true},
}

func init() {
	for _, p := range compatProviders {
		provider.Register(provider.Info{
			Name:          p.name,
			DisplayName:   p.displayName,
			DefaultEnvVar: p.defaultEnvVar,
			NoAuth:        p.noAuth,
			Factory: func(cfg provider.Config) (provider.AgentClient, error) {
				return New(p.name, p.defaultURL, cfg)
			},
		})
	}
}

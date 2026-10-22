package anthropic

import "github.com/matteo-psnt/termwise/internal/provider"

// displayName names the provider in user-facing errors and the registry.
const displayName = "Anthropic"

func init() {
	provider.Register(provider.Info{
		Name:          "anthropic",
		DisplayName:   displayName,
		DefaultEnvVar: "ANTHROPIC_API_KEY",
		Factory: func(cfg provider.Config) (provider.AgentClient, error) {
			return New(cfg)
		},
	})
}

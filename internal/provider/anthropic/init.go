package anthropic

import "github.com/matteo-psnt/termwise/internal/provider"

func init() {
	provider.Register(provider.Info{
		Name:          "anthropic",
		DisplayName:   "Anthropic",
		DefaultEnvVar: "ANTHROPIC_API_KEY",
		Factory: func(cfg provider.Config) (provider.AgentClient, error) {
			return New(cfg)
		},
	})
}

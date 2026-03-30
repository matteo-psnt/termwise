package config

import (
	"context"
	"fmt"

	"github.com/matteo-psnt/termwise/internal/ai"
)

// LoadRuntimeConfig loads config from disk, falling back to zero-config defaults.
func LoadRuntimeConfig(path string) (Config, error) {
	cfg, exists, err := LoadConfig(path)
	if err != nil {
		return Config{}, err
	}
	if !exists {
		var ok bool
		cfg, ok = ZeroConfigDefaults()
		if !ok {
			return Config{}, fmt.Errorf("no configuration found — run `tw config` to set up")
		}
	}
	if err := ValidateForRuntime(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// NewAgentProvider resolves auth and constructs an AI provider for the config block.
func NewAgentProvider(name string, pc ProviderConfig) (ai.AgentProvider, error) {
	return NewAgentProviderForModel(name, pc, pc.Model)
}

// NewAgentProviderForModel resolves auth and constructs an AI provider.
func NewAgentProviderForModel(name string, pc ProviderConfig, model string) (ai.AgentProvider, error) {
	auth, err := ResolveAuth(name, pc)
	if err != nil {
		return nil, err
	}
	return ai.GetProvider(name, ai.ProviderConfig{
		APIKey:  auth.APIKey,
		Model:   model,
		BaseURL: auth.BaseURL,
	})
}

// ListProviderModels lists the models available for a configured provider.
func ListProviderModels(ctx context.Context, name string, pc ProviderConfig) ([]ai.Model, error) {
	provider, err := NewAgentProvider(name, pc)
	if err != nil {
		return nil, err
	}
	return provider.ListModels(ctx)
}

// CheckProviderConnectivity verifies the provider can be reached and returns models.
func CheckProviderConnectivity(ctx context.Context, name string, pc ProviderConfig) error {
	models, err := ListProviderModels(ctx, name, pc)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return fmt.Errorf("no models returned by %s", name)
	}
	return nil
}

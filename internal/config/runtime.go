package config

import (
	"context"
	"fmt"

	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/provider/anthropic"
	"github.com/matteo-psnt/termwise/internal/provider/openaicompat"
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

// NewProviderClient resolves auth and constructs a provider client for the config block.
func NewProviderClient(name string, pc ProviderConfig) (provider.AgentClient, error) {
	return NewProviderClientForModel(name, pc, pc.Model)
}

// NewProviderClientForModel resolves auth and constructs a provider client.
func NewProviderClientForModel(name string, pc ProviderConfig, model string) (provider.AgentClient, error) {
	auth, err := ResolveAuth(name, pc)
	if err != nil {
		return nil, err
	}
	cfg := provider.Config{
		APIKey:  auth.APIKey,
		Model:   model,
		BaseURL: auth.BaseURL,
	}
	switch name {
	case "anthropic":
		return anthropic.New(cfg)
	default:
		baseURL, ok := openaicompat.DefaultBaseURL(name)
		if !ok {
			return nil, fmt.Errorf("unknown provider %q — supported: %v", name, ProviderNames())
		}
		return openaicompat.New(name, baseURL, cfg)
	}
}

// ListProviderModels lists the models available for a configured provider.
func ListProviderModels(ctx context.Context, name string, pc ProviderConfig) ([]provider.Model, error) {
	client, err := NewProviderClient(name, pc)
	if err != nil {
		return nil, err
	}
	return client.ListModels(ctx)
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

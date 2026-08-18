package config

import (
	"context"
	"fmt"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// ListProviderModels returns the models available for the given provider config.
func ListProviderModels(ctx context.Context, name string, pc ProviderConfig) ([]provider.Model, error) {
	auth, err := ResolveAuth(name, pc)
	if err != nil {
		return nil, err
	}
	desc := modelCacheDescriptorFor(name, auth)
	return defaultProviderModelsCache.getOrFetch(ctx, desc, func(ctx context.Context) ([]provider.Model, error) {
		client, err := newProviderClientWithAuth(name, pc, auth)
		if err != nil {
			return nil, err
		}
		return client.ListModels(ctx)
	})
}

// CheckProviderConnectivity verifies that the provider is reachable and returns
// at least one model. Returns nil on success.
func CheckProviderConnectivity(ctx context.Context, name string, pc ProviderConfig) error {
	ms, err := ListProviderModels(ctx, name, pc)
	if err != nil {
		return err
	}
	if len(ms) == 0 {
		return fmt.Errorf("no models returned by %s", name)
	}
	return nil
}

func newProviderClientWithAuth(name string, pc ProviderConfig, auth ResolvedAuth) (provider.AgentClient, error) {
	return provider.NewClient(name, provider.Config{
		APIKey:  auth.APIKey,
		Model:   pc.Model,
		BaseURL: auth.BaseURL,
	})
}

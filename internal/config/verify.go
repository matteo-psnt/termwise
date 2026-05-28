package config

import (
	"context"
	"fmt"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// ListProviderModels returns the models available for the given provider config.
func ListProviderModels(ctx context.Context, name string, pc ProviderConfig) ([]provider.Model, error) {
	client, err := newProviderClient(name, pc)
	if err != nil {
		return nil, err
	}
	return client.ListModels(ctx)
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

// newProviderClient resolves auth and constructs a provider client from a
// ProviderConfig. Used internally for connectivity checks and model listing.
func newProviderClient(name string, pc ProviderConfig) (provider.AgentClient, error) {
	auth, err := ResolveAuth(name, pc)
	if err != nil {
		return nil, err
	}
	if name != "anthropic" && auth.BaseURL == "" && !isKnownProvider(name) {
		return nil, fmt.Errorf("unknown provider %q — supported: %v", name, ProviderNames())
	}
	return newClient(name, provider.Config{
		APIKey:  auth.APIKey,
		Model:   pc.Model,
		BaseURL: auth.BaseURL,
	})
}

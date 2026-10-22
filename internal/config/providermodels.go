package config

import (
	"context"
	"fmt"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// ListProviderModels returns the models available for the given provider
// config, which need not be saved yet: the config UI and wizard use it to
// fill model pickers. Results are cached per provider, key and URL, and a
// cached list can be days old.
func ListProviderModels(ctx context.Context, name string, pc ProviderConfig) ([]provider.Model, error) {
	return providerModels(ctx, name, pc, false)
}

// FetchProviderModels is ListProviderModels without the cache: it always asks
// the provider, then caches the answer. The config commands and doctor use
// it, since they report on the provider as it is now — a cached list would
// pass a check against a server that is down or a key that was revoked, and
// reject a model released since the list was cached.
func FetchProviderModels(ctx context.Context, name string, pc ProviderConfig) ([]provider.Model, error) {
	return providerModels(ctx, name, pc, true)
}

func providerModels(ctx context.Context, name string, pc ProviderConfig, fresh bool) ([]provider.Model, error) {
	auth, err := ResolveAuth(name, pc)
	if err != nil {
		return nil, err
	}
	desc := modelCacheDescriptorFor(name, auth)
	fetch := func(ctx context.Context) ([]provider.Model, error) {
		client, err := newProviderClientWithAuth(name, pc, auth)
		if err != nil {
			return nil, err
		}
		return client.ListModels(ctx)
	}
	if fresh {
		return defaultProviderModelsCache.fetchFresh(ctx, desc, fetch)
	}
	return defaultProviderModelsCache.getOrFetch(ctx, desc, fetch)
}

// CheckProviderConnectivity verifies that the provider is reachable now and
// returns at least one model. Returns nil on success.
func CheckProviderConnectivity(ctx context.Context, name string, pc ProviderConfig) error {
	ms, err := FetchProviderModels(ctx, name, pc)
	if err != nil {
		return err
	}
	if len(ms) == 0 {
		return fmt.Errorf("no models returned by %s", name)
	}
	return nil
}

func newProviderClientWithAuth(name string, pc ProviderConfig, auth ResolvedAuth) (provider.AgentClient, error) {
	return NewClientFromResolved(ResolvedConfig{
		ProviderName: name,
		Model:        pc.Model,
		APIKey:       auth.APIKey,
		BaseURL:      auth.BaseURL,
		KeySource:    auth.Source,
	})
}

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
	ms, _, err := providerModels(ctx, name, pc, false)
	return ms, err
}

// FetchProviderModels is ListProviderModels without the cache: it always asks
// the provider, then caches the answer. The config commands and doctor use
// it, since they report on the provider as it is now — a cached list would
// pass a check against a server that is down or a key that was revoked, and
// reject a model released since the list was cached.
func FetchProviderModels(ctx context.Context, name string, pc ProviderConfig) ([]provider.Model, error) {
	ms, _, err := providerModels(ctx, name, pc, true)
	return ms, err
}

func providerModels(ctx context.Context, name string, pc ProviderConfig, fresh bool) ([]provider.Model, ResolvedAuth, error) {
	auth, err := ResolveAuth(name, pc)
	if err != nil {
		return nil, auth, err
	}
	desc := modelCacheDescriptorFor(name, auth)
	fetch := func(ctx context.Context) ([]provider.Model, error) {
		client, err := newProviderClientWithAuth(name, pc, auth)
		if err != nil {
			return nil, err
		}
		return client.ListModels(ctx)
	}
	var ms []provider.Model
	if fresh {
		ms, err = defaultProviderModelsCache.fetchFresh(ctx, desc, fetch)
	} else {
		ms, err = defaultProviderModelsCache.getOrFetch(ctx, desc, fetch)
	}
	return ms, auth, err
}

// CheckProviderConnectivity verifies that the provider is reachable now and
// returns at least one model. Returns nil on success.
func CheckProviderConnectivity(ctx context.Context, name string, pc ProviderConfig) error {
	_, err := ProbeProvider(ctx, name, pc)
	return err
}

// ProbeProvider is CheckProviderConnectivity that also returns the auth it
// used, so a caller can tell when the key came from ResolveAuth's fallback
// rather than the configured method. Resolving auth separately would run
// api_key_cmd, and any unlock prompt it shows, a second time.
func ProbeProvider(ctx context.Context, name string, pc ProviderConfig) (ResolvedAuth, error) {
	ms, auth, err := providerModels(ctx, name, pc, true)
	if err != nil {
		return auth, err
	}
	if len(ms) == 0 {
		return auth, fmt.Errorf("no models returned by %s", name)
	}
	return auth, nil
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

package configui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// serverURL is a self-hosted provider's default address.
func serverURL(providerName string) string {
	info, _ := provider.Lookup(providerName)
	return info.ServerURL
}

// buildProviderConfigFromAuthInput turns what the user typed for method into
// the ProviderConfig to save. For keychain the input is the raw API key, which
// must never reach the config file: the entry name is always the provider's
// default, the one StoreKeychain writes to.
func buildProviderConfigFromAuthInput(providerName, method, inputVal, fallback string) config.ProviderConfig {
	// Pasted values often carry a trailing newline or stray spaces, which
	// would end up in the env var name, command or URL.
	val := strings.TrimSpace(inputVal)
	if val == "" {
		val = fallback
	}

	pc := config.ProviderConfig{Auth: method}
	switch {
	case provider.HasNoAuth(providerName):
		pc.Auth = "env"
		if val != "" && val != serverURL(providerName) {
			pc.BaseURL = val
		}
	case method == "env":
		pc.EnvVar = val
	case method == "cmd":
		pc.APIKeyCmd = val
	case method == "keychain":
		pc.KeychainEntry = config.DefaultKeychainEntry(providerName)
	}

	return pc
}

// listModelsFor lists the models available with pc, failing if there are
// none. For keychain auth, apiKey is the key just typed in: it is tried
// directly rather than read back from the keychain, and stored only once the
// provider accepts it, so a mistyped key never replaces one that works. The
// write goes through journal so an editor session can undo it.
func listModelsFor(ctx context.Context, providerName string, pc config.ProviderConfig, apiKey string, journal *keychainJournal) ([]provider.Model, error) {
	apiKey = strings.TrimSpace(apiKey)
	if pc.AuthMethod() == "keychain" && apiKey == "" {
		return nil, errors.New("API key cannot be empty")
	}
	var list []provider.Model
	var err error
	if pc.AuthMethod() == "keychain" {
		list, err = listModelsWithKey(ctx, providerName, pc, apiKey)
	} else {
		// This is a check that the settings work now, so ask the provider:
		// the model cache can be days old, and would "verify" a revoked key
		// or a server that is down.
		list, err = config.FetchProviderModels(ctx, providerName, pc)
	}
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no models returned by %s", providerName)
	}
	if pc.AuthMethod() == "keychain" {
		if err := journal.store(providerName, apiKey); err != nil {
			return nil, fmt.Errorf("keychain write: %w", err)
		}
	}
	return list, nil
}

// listModelsWithKey lists pc's models authenticating with apiKey, bypassing
// pc's auth method (and the model cache, which is keyed by resolved auth).
func listModelsWithKey(ctx context.Context, providerName string, pc config.ProviderConfig, apiKey string) ([]provider.Model, error) {
	client, err := config.NewClientFromResolved(config.ResolvedConfig{
		ProviderName: providerName,
		Model:        pc.Model,
		APIKey:       apiKey,
		BaseURL:      pc.BaseURL,
	})
	if err != nil {
		return nil, err
	}
	return client.ListModels(ctx)
}

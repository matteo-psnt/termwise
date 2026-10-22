package configui

import (
	"fmt"

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
	val := inputVal
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

// prepareProviderConfig is buildProviderConfigFromAuthInput for a config that
// is about to be used: for keychain, inputVal is the raw API key and has to be
// in the keychain before anything can authenticate with the result.
func prepareProviderConfig(providerName, method, inputVal, fallback string) (config.ProviderConfig, error) {
	if method == "keychain" {
		if err := config.StoreKeychain(providerName, inputVal); err != nil {
			return config.ProviderConfig{}, fmt.Errorf("keychain write: %w", err)
		}
	}
	return buildProviderConfigFromAuthInput(providerName, method, inputVal, fallback), nil
}

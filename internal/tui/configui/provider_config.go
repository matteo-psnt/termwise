package configui

import (
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// serverURL is a self-hosted provider's default address.
func serverURL(providerName string) string {
	info, _ := provider.Lookup(providerName)
	return info.ServerURL
}

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
		pc.KeychainEntry = val
	}

	return pc
}

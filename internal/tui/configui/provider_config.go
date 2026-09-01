package configui

import (
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
)

const defaultOllamaBaseURL = "http://localhost:11434"

func buildProviderConfigFromAuthInput(providerName, method, inputVal, fallback string) config.ProviderConfig {
	val := inputVal
	if val == "" {
		val = fallback
	}

	pc := config.ProviderConfig{Auth: method}
	switch {
	case provider.HasNoAuth(providerName):
		pc.Auth = "env"
		if val != "" && val != defaultOllamaBaseURL {
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

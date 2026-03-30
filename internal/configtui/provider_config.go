package configtui

import "github.com/matteo-psnt/termwise/internal/config"

const defaultOllamaBaseURL = "http://localhost:11434"

func buildProviderConfigFromAuthInput(provider, method, inputVal, fallback string) config.ProviderConfig {
	val := inputVal
	if val == "" {
		val = fallback
	}

	pc := config.ProviderConfig{AuthMethod: method}
	switch {
	case provider == "ollama":
		pc.AuthMethod = "env"
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

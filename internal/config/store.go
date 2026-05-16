package config

import (
	"fmt"
	"strings"
)

// UseProvider sets the selected provider.
// Returns an error if name is not in the known provider catalog.
func UseProvider(cfg *FileConfig, name string) error {
	if !isKnownProvider(name) {
		return fmt.Errorf(
			"unknown provider %q\n  Supported: %s",
			name, strings.Join(ProviderNames(), ", "),
		)
	}
	cfg.SelectedProvider = name
	return nil
}

// SetModel sets the model for the named provider, creating the provider block
// if one does not yet exist.
func SetModel(cfg *FileConfig, providerName, model string) error {
	if model == "" {
		return fmt.Errorf("model cannot be empty")
	}
	pc := cfg.Providers[providerName]
	pc.Model = model
	SetProvider(cfg, providerName, pc)
	return nil
}

// SetAuthEnv configures environment-variable auth for a provider.
// Any previously configured cmd or keychain fields are cleared.
func SetAuthEnv(cfg *FileConfig, providerName, envVar string) error {
	if envVar == "" {
		return fmt.Errorf("env_var cannot be empty")
	}
	pc := cfg.Providers[providerName]
	pc.Auth = "env"
	pc.EnvVar = envVar
	pc.APIKeyCmd = ""
	pc.KeychainEntry = ""
	SetProvider(cfg, providerName, pc)
	return nil
}

// SetAuthCmd configures shell-command auth for a provider.
// Any previously configured env or keychain fields are cleared.
func SetAuthCmd(cfg *FileConfig, providerName, cmd string) error {
	if cmd == "" {
		return fmt.Errorf("api_key_cmd cannot be empty")
	}
	pc := cfg.Providers[providerName]
	pc.Auth = "cmd"
	pc.APIKeyCmd = cmd
	pc.EnvVar = ""
	pc.KeychainEntry = ""
	SetProvider(cfg, providerName, pc)
	return nil
}

// SetAuthKeychain configures keychain auth for a provider.
// Any previously configured env or cmd fields are cleared.
func SetAuthKeychain(cfg *FileConfig, providerName, entry string) error {
	if entry == "" {
		return fmt.Errorf("keychain_entry cannot be empty")
	}
	pc := cfg.Providers[providerName]
	pc.Auth = "keychain"
	pc.KeychainEntry = entry
	pc.EnvVar = ""
	pc.APIKeyCmd = ""
	SetProvider(cfg, providerName, pc)
	return nil
}

// SetProvider replaces the config block for providerName, creating the
// providers map if needed.
func SetProvider(cfg *FileConfig, name string, pc ProviderConfig) {
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}
	cfg.Providers[name] = pc
}

// RemoveProvider removes the config block for name.
// Returns an error if name is the currently selected provider.
func RemoveProvider(cfg *FileConfig, name string) error {
	if cfg.SelectedProvider == name {
		return fmt.Errorf(
			"cannot remove %q — it is the selected provider\n  Run `tw config use <provider>` first",
			name,
		)
	}
	delete(cfg.Providers, name)
	return nil
}

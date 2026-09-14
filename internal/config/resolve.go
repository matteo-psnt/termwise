package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/matteo-psnt/termwise/internal/cliname"

	"github.com/zalando/go-keyring"

	"github.com/matteo-psnt/termwise/internal/models"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// ResolvedConfig is the fully-resolved provider configuration the runtime operates with.
// It contains auth credentials and provider identity — concrete values only, no optional
// pointers or env var names. User preferences (theme, keybinding, etc.) are read directly
// from FileConfig via the Settings registry; they do not belong here.
type ResolvedConfig struct {
	ProviderName string
	Model        string
	APIKey       string
	BaseURL      string
}

// ResolvedAuth holds the resolved API key and an optional base URL override.
type ResolvedAuth struct {
	APIKey  string
	BaseURL string
}

// Resolve derives a ResolvedConfig from a FileConfig.
// It resolves auth credentials for the selected provider but makes no network
// calls. Returns an error if the selected provider is missing or auth fails.
func Resolve(cfg FileConfig) (ResolvedConfig, error) {
	name := cfg.SelectedProvider
	if name == "" {
		return ResolvedConfig{}, fmt.Errorf("no provider selected — run `%s config` to set up", cliname.Name())
	}

	pc, ok := cfg.Providers[name]
	if !ok {
		return ResolvedConfig{}, fmt.Errorf(
			"provider %q has no config block\n  Add a [providers.%s] block or run `%s config`",
			name, name, cliname.Name(),
		)
	}

	auth, err := ResolveAuth(name, pc)
	if err != nil {
		return ResolvedConfig{}, fmt.Errorf("resolving auth for %q: %w", name, err)
	}

	return ResolvedConfig{
		ProviderName: name,
		Model:        pc.Model,
		APIKey:       auth.APIKey,
		BaseURL:      auth.BaseURL,
	}, nil
}

// LoadAndResolve loads, validates, and resolves the config at path, returning
// both the raw FileConfig (for settings access) and the resolved runtime config.
func LoadAndResolve(path string) (FileConfig, ResolvedConfig, error) {
	cfg, exists, err := LoadConfig(path)
	if err != nil {
		return FileConfig{}, ResolvedConfig{}, err
	}
	if !exists {
		var ok bool
		cfg, ok = ZeroConfigDefaults()
		if !ok {
			return FileConfig{}, ResolvedConfig{}, fmt.Errorf("no configuration found — run `%s config` to set up", cliname.Name())
		}
	}
	if err := ValidateForRuntime(cfg); err != nil {
		return FileConfig{}, ResolvedConfig{}, err
	}
	rc, err := Resolve(cfg)
	return cfg, rc, err
}

// LoadRuntimeConfig loads the config file, falls back to zero-config defaults
// when the file is absent, validates it structurally, and resolves it into a
// ResolvedConfig. This is the entry point for the normal runtime path.
func LoadRuntimeConfig(path string) (ResolvedConfig, error) {
	_, rc, err := LoadAndResolve(path)
	return rc, err
}

// ZeroConfigDefaults builds a minimal FileConfig from well-known API key
// environment variables when no config file exists. It returns the first
// provider whose env var is set, or (FileConfig{}, false) if none match.
// Ollama is excluded — it needs no API key and cannot be auto-detected.
func ZeroConfigDefaults() (FileConfig, bool) {
	for _, info := range provider.Catalog() {
		if info.DefaultEnvVar == "" || os.Getenv(info.DefaultEnvVar) == "" {
			continue
		}
		return FileConfig{
			SelectedProvider: info.Name,
			Providers: map[string]ProviderConfig{
				info.Name: {
					Auth:   "env",
					EnvVar: info.DefaultEnvVar,
					Model:  models.DefaultModel(info.Name),
				},
			},
		}, true
	}
	return FileConfig{}, false
}

// NewClientFromResolved constructs a provider client from a ResolvedConfig.
// Auth credentials must already be resolved in rc.
func NewClientFromResolved(rc ResolvedConfig) (provider.AgentClient, error) {
	return provider.NewClient(rc.ProviderName, provider.Config{
		APIKey:  rc.APIKey,
		Model:   rc.Model,
		BaseURL: rc.BaseURL,
	})
}

// ResolveAuth returns the resolved API key and base URL for a provider config.
// If the configured auth method fails, it tries the provider's well-known env
// var as a fallback before returning an error.
func ResolveAuth(name string, pc ProviderConfig) (ResolvedAuth, error) {
	key, err := resolveKey(name, pc)
	if err != nil {
		if fb := DefaultEnvVar(name); fb != "" {
			if v := os.Getenv(fb); v != "" {
				return ResolvedAuth{APIKey: v, BaseURL: pc.BaseURL}, nil
			}
		}
		return ResolvedAuth{}, err
	}
	return ResolvedAuth{APIKey: key, BaseURL: pc.BaseURL}, nil
}

func resolveKey(name string, pc ProviderConfig) (string, error) {
	method := pc.Auth
	if method == "" {
		method = "env"
	}

	switch method {
	case "env":
		envVar := pc.EnvVar
		if envVar == "" {
			envVar = DefaultEnvVar(name)
		}
		if envVar == "" {
			// Provider requires no key (e.g. Ollama local).
			return "", nil
		}
		val := os.Getenv(envVar)
		if val == "" {
			return "", fmt.Errorf("environment variable %q is not set", envVar)
		}
		return val, nil

	case "cmd":
		if pc.APIKeyCmd == "" {
			return "", fmt.Errorf("auth is \"cmd\" but api_key_cmd is not set in [providers.%s]", name)
		}
		out, err := exec.Command("sh", "-c", pc.APIKeyCmd).Output()
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
				return "", fmt.Errorf("%s", strings.TrimSpace(string(exitErr.Stderr)))
			}
			return "", fmt.Errorf("api_key_cmd failed: %w", err)
		}
		key := strings.TrimSpace(string(out))
		if key == "" {
			return "", fmt.Errorf("api_key_cmd produced no output")
		}
		return key, nil

	case "keychain":
		if pc.KeychainEntry == "" {
			return "", fmt.Errorf("auth is \"keychain\" but keychain_entry is not set in [providers.%s]", name)
		}
		key, err := keyring.Get("termwise", pc.KeychainEntry)
		if err != nil {
			return "", fmt.Errorf("keychain entry %q not found", pc.KeychainEntry)
		}
		return key, nil

	default:
		return "", fmt.Errorf(
			"unknown auth %q in [providers.%s]\n  Valid methods: env, cmd, keychain",
			method, name,
		)
	}
}

// DefaultKeychainEntry returns the conventional keychain entry name for a provider.
func DefaultKeychainEntry(providerName string) string {
	return "termwise-" + providerName
}

// StoreKeychain writes an API key into the macOS Keychain under the termwise service.
func StoreKeychain(providerName, apiKey string) error {
	return keyring.Set("termwise", DefaultKeychainEntry(providerName), apiKey)
}

// ListProviderModels returns the models available for the given provider config.
// Used by the config UI / wizard to populate model pickers before saving.
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

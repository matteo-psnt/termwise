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
	// KeySource says where APIKey came from, for error messages:
	// "$OPENAI_API_KEY", "api_key_cmd", or a keychain entry. Empty when the
	// provider needs no key.
	KeySource string
}

// ResolvedAuth holds the resolved API key and an optional base URL override.
type ResolvedAuth struct {
	APIKey  string
	BaseURL string
	Source  string // see ResolvedConfig.KeySource
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
		KeySource:    auth.Source,
	}, nil
}

// ErrNoConfig is returned when there is no config file and no API key in the
// environment to fall back on. Interactive entry points use it to start setup.
var ErrNoConfig = errors.New("no configuration found")

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
		if !ok && os.Getenv(EnvProvider) == "" {
			return FileConfig{}, ResolvedConfig{}, fmt.Errorf("%w — run `%s config` to set up, or export an API key such as ANTHROPIC_API_KEY or OPENAI_API_KEY", ErrNoConfig, cliname.Name())
		}
	}
	if err := applyEnvOverrides(&cfg); err != nil {
		return FileConfig{}, ResolvedConfig{}, err
	}
	if err := ValidateForRuntime(cfg); err != nil {
		return FileConfig{}, ResolvedConfig{}, err
	}
	rc, err := Resolve(cfg)
	return cfg, rc, err
}

// Environment variables that override the config file for one run, so a
// script or CI job can pick a provider and model without writing config.
const (
	EnvProvider = "TERMWISE_PROVIDER"
	EnvModel    = "TERMWISE_MODEL"
)

// applyEnvOverrides applies TERMWISE_PROVIDER and TERMWISE_MODEL to cfg in
// memory. A provider with no block in the file gets the same defaults
// `config use` would write.
func applyEnvOverrides(cfg *FileConfig) error {
	if name := os.Getenv(EnvProvider); name != "" {
		if err := UseProvider(cfg, name); err != nil {
			return fmt.Errorf("%s: %w", EnvProvider, err)
		}
	}
	if model := os.Getenv(EnvModel); model != "" && cfg.SelectedProvider != "" {
		pc := cfg.Providers[cfg.SelectedProvider]
		pc.Model = model
		SetProvider(cfg, cfg.SelectedProvider, pc)
	}
	return nil
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

// EnvOnlyNotice describes the zero-config fallback when it is in effect:
// there is no file at path, so the provider comes from an API key in the
// environment. It returns "" when a config file exists or no key is set.
func EnvOnlyNotice(path string) string {
	if _, err := os.Stat(path); err == nil {
		return ""
	}
	cfg, ok := ZeroConfigDefaults()
	if !ok {
		return ""
	}
	name := cfg.SelectedProvider
	pc := cfg.Providers[name]
	return fmt.Sprintf("No config file — using %s via $%s (model %s). Run `%s config` to save or change this.",
		displayName(name), pc.EnvVar, pc.Model, cliname.Name())
}

// NewClientFromResolved constructs a provider client from a ResolvedConfig.
// Auth credentials must already be resolved in rc.
func NewClientFromResolved(rc ResolvedConfig) (provider.AgentClient, error) {
	client, err := provider.NewClient(rc.ProviderName, provider.Config{
		APIKey:  rc.APIKey,
		Model:   rc.Model,
		BaseURL: rc.BaseURL,
	})
	if err != nil {
		return nil, err
	}
	return withErrorHints(client, rc), nil
}

// ResolveAuth returns the resolved API key and base URL for a provider config.
// If the configured auth method fails, it tries the provider's well-known env
// var as a fallback before returning an error.
func ResolveAuth(name string, pc ProviderConfig) (ResolvedAuth, error) {
	key, source, err := resolveKey(name, pc)
	if err != nil {
		if fb := DefaultEnvVar(name); fb != "" {
			if v := os.Getenv(fb); v != "" {
				return ResolvedAuth{APIKey: v, BaseURL: pc.BaseURL, Source: "$" + fb}, nil
			}
		}
		return ResolvedAuth{}, err
	}
	return ResolvedAuth{APIKey: key, BaseURL: pc.BaseURL, Source: source}, nil
}

// resolveKey returns the API key for pc and a description of where it came
// from.
func resolveKey(name string, pc ProviderConfig) (string, string, error) {
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
			return "", "", nil
		}
		val := os.Getenv(envVar)
		if val == "" {
			return "", "", fmt.Errorf("environment variable %q is not set", envVar)
		}
		return val, "$" + envVar, nil

	case "cmd":
		if pc.APIKeyCmd == "" {
			return "", "", fmt.Errorf("auth is \"cmd\" but api_key_cmd is not set in [providers.%s]", name)
		}
		out, err := exec.Command("sh", "-c", pc.APIKeyCmd).Output()
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
				return "", "", fmt.Errorf("%s", strings.TrimSpace(string(exitErr.Stderr)))
			}
			return "", "", fmt.Errorf("api_key_cmd failed: %w", err)
		}
		key := strings.TrimSpace(string(out))
		if key == "" {
			return "", "", fmt.Errorf("api_key_cmd produced no output")
		}
		return key, "api_key_cmd", nil

	case "keychain":
		if pc.KeychainEntry == "" {
			return "", "", fmt.Errorf("auth is \"keychain\" but keychain_entry is not set in [providers.%s]", name)
		}
		key, err := keyring.Get("termwise", pc.KeychainEntry)
		if err != nil {
			return "", "", fmt.Errorf("keychain entry %q not found — run `%s config` to store a key", pc.KeychainEntry, cliname.Name())
		}
		return key, fmt.Sprintf("keychain entry %q", pc.KeychainEntry), nil

	default:
		return "", "", fmt.Errorf(
			"unknown auth %q in [providers.%s]\n  Valid methods: env, cmd, keychain",
			method, name,
		)
	}
}

// DefaultKeychainEntry returns the conventional keychain entry name for a provider.
func DefaultKeychainEntry(providerName string) string {
	return "termwise-" + providerName
}

// StoreKeychain writes an API key into the system keychain under the termwise
// service, at the provider's default entry.
func StoreKeychain(providerName, apiKey string) error {
	return StoreKeychainEntry(DefaultKeychainEntry(providerName), apiKey)
}

// StoreKeychainEntry writes an API key into the system keychain under the
// termwise service.
func StoreKeychainEntry(entry, apiKey string) error {
	return keyring.Set("termwise", entry, apiKey)
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
	client, err := provider.NewClient(name, provider.Config{
		APIKey:  auth.APIKey,
		Model:   pc.Model,
		BaseURL: auth.BaseURL,
	})
	if err != nil {
		return nil, err
	}
	return withErrorHints(client, ResolvedConfig{
		ProviderName: name,
		Model:        pc.Model,
		BaseURL:      auth.BaseURL,
		KeySource:    auth.Source,
	}), nil
}

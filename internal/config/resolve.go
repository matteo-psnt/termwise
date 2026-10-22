package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/matteo-psnt/termwise/internal/cliname"
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

// resolveConfig derives a ResolvedConfig from a FileConfig.
// It resolves auth credentials for the selected provider but makes no network
// calls. Returns an error if the selected provider is missing or auth fails.
func resolveConfig(cfg FileConfig) (ResolvedConfig, error) {
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
	if err := validateForRuntime(cfg); err != nil {
		return FileConfig{}, ResolvedConfig{}, err
	}
	rc, err := resolveConfig(cfg)
	return cfg, rc, err
}

// LoadRuntimeConfig loads the config file, falls back to zero-config defaults
// when the file is absent, validates it structurally, and resolves it into a
// ResolvedConfig. This is the entry point for the normal runtime path.
func LoadRuntimeConfig(path string) (ResolvedConfig, error) {
	_, rc, err := LoadAndResolve(path)
	return rc, err
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

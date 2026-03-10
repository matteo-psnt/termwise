package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// ProviderConfig holds per-provider settings as stored in config.toml.
type ProviderConfig struct {
	// Auth
	AuthMethod   string `toml:"auth_method,omitempty"`
	EnvVar       string `toml:"env_var,omitempty"`
	APIKeyCmd    string `toml:"api_key_cmd,omitempty"`
	KeychainEntry string `toml:"keychain_entry,omitempty"`

	// Model and endpoint
	Model   string `toml:"model,omitempty"`
	BaseURL string `toml:"base_url,omitempty"`
}

// TUIConfig holds TUI display settings.
type TUIConfig struct {
	ShowFooter *bool `toml:"show_footer,omitempty"`
}

// ShellConfig holds shell integration settings.
type ShellConfig struct {
	Keybinding string `toml:"keybinding,omitempty"`
}

// Config is the in-memory representation of config.toml.
type Config struct {
	ActiveProvider string                    `toml:"active_provider,omitempty"`
	Providers      map[string]ProviderConfig `toml:"providers,omitempty"`
	TUI            TUIConfig                 `toml:"tui,omitempty"`
	Shell          ShellConfig               `toml:"shell,omitempty"`
}

// DefaultConfigPath returns ~/.config/termwise/config.toml.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "termwise", "config.toml"), nil
}

// configDir returns the directory containing the config file.
func configDir(cfgPath string) string {
	return filepath.Dir(cfgPath)
}

// LoadConfig reads and parses the config file at path.
// If the file does not exist, it returns an empty Config and exists=false with no error.
// Any other error (parse failure, permission issue) is returned as an error.
func LoadConfig(path string) (cfg Config, exists bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, false, nil
		}
		return Config{}, false, fmt.Errorf("could not read config file: %w", err)
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, true, fmt.Errorf("config file is invalid: %w\n  File: %s", err, path)
	}

	return cfg, true, nil
}

// SaveConfig writes cfg to path atomically (write to temp file, rename).
// Creates the config directory with 0700 if it does not exist.
func SaveConfig(path string, cfg Config) error {
	dir := configDir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("could not create config directory: %w", err)
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("could not marshal config: %w", err)
	}

	// Write to a temp file in the same directory, then rename for atomicity.
	tmp, err := os.CreateTemp(dir, ".config.toml.tmp")
	if err != nil {
		return fmt.Errorf("could not create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("could not write config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("could not close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("could not set config permissions: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("could not save config: %w", err)
	}

	return nil
}

// ZeroConfigDefaults attempts to build a usable Config from environment variables
// when no config file exists. Returns the config and whether a key was found.
//
// Priority:
//  1. TERMWISE_API_KEY → anthropic
//  2. ANTHROPIC_API_KEY → anthropic
//  3. OPENAI_API_KEY → openai
func ZeroConfigDefaults() (Config, bool) {
	if key := os.Getenv("TERMWISE_API_KEY"); key != "" {
		return envKeyConfig("anthropic", "TERMWISE_API_KEY", defaultModel("anthropic")), true
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return envKeyConfig("anthropic", "ANTHROPIC_API_KEY", defaultModel("anthropic")), true
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return envKeyConfig("openai", "OPENAI_API_KEY", defaultModel("openai")), true
	}
	return Config{}, false
}

func envKeyConfig(provider, envVar, model string) Config {
	return Config{
		ActiveProvider: provider,
		Providers: map[string]ProviderConfig{
			provider: {
				AuthMethod: "env",
				EnvVar:     envVar,
				Model:      model,
			},
		},
	}
}

// defaultModel returns the default model ID for a known provider.
func defaultModel(provider string) string {
	switch provider {
	case "anthropic":
		return "claude-haiku-4-5-20251001"
	case "openai":
		return "gpt-4o-mini"
	default:
		return ""
	}
}

// ActiveProviderConfig returns the ProviderConfig for the active provider.
// Returns an error if the active provider is not set or has no config block.
func (c Config) ActiveProviderConfig() (string, ProviderConfig, error) {
	if c.ActiveProvider == "" {
		return "", ProviderConfig{}, fmt.Errorf("no active provider set — run `tw config` to set up")
	}
	pc, ok := c.Providers[c.ActiveProvider]
	if !ok {
		return "", ProviderConfig{}, fmt.Errorf(
			"provider %q is set as active but has no config\n  Add a [providers.%s] block or run `tw config`",
			c.ActiveProvider, c.ActiveProvider,
		)
	}
	return c.ActiveProvider, pc, nil
}

// SetProvider sets or replaces a provider config block.
func (c *Config) SetProvider(name string, pc ProviderConfig) {
	if c.Providers == nil {
		c.Providers = make(map[string]ProviderConfig)
	}
	c.Providers[name] = pc
}

package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/matteo-psnt/termwise/internal/models"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/provider/anthropic"
	"github.com/matteo-psnt/termwise/internal/provider/openaicompat"
	"github.com/matteo-psnt/termwise/internal/theme"
	"github.com/zalando/go-keyring"
)

// ResolvedConfig is the fully-resolved configuration the runtime operates with.
// It is derived from FileConfig and contains concrete values only — no optional
// pointers, no auth method selectors, no env var names.
type ResolvedConfig struct {
	ProviderName    string
	Model           string
	APIKey          string
	BaseURL         string
	Theme           string
	ShowFooter      bool
	LLMJudge        bool
	ShellKeybinding string
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
		return ResolvedConfig{}, fmt.Errorf("no provider selected — run `tw config` to set up")
	}

	pc, ok := cfg.Providers[name]
	if !ok {
		return ResolvedConfig{}, fmt.Errorf(
			"provider %q has no config block\n  Add a [providers.%s] block or run `tw config`",
			name, name,
		)
	}

	auth, err := ResolveAuth(name, pc)
	if err != nil {
		return ResolvedConfig{}, fmt.Errorf("resolving auth for %q: %w", name, err)
	}

	rc := ResolvedConfig{
		ProviderName:    name,
		Model:           pc.Model,
		APIKey:          auth.APIKey,
		BaseURL:         auth.BaseURL,
		Theme:           theme.Normalize(cfg.UI.Theme),
		ShowFooter:      true,
		LLMJudge:        cfg.Policies.Bash.LLMJudge,
		ShellKeybinding: cfg.Shell.Keybinding,
	}
	if cfg.UI.ShowFooter != nil {
		rc.ShowFooter = *cfg.UI.ShowFooter
	}
	if rc.Theme == "" {
		rc.Theme = theme.DefaultName
	}
	if rc.ShellKeybinding == "" {
		rc.ShellKeybinding = "^T"
	}
	return rc, nil
}

// LoadRuntimeConfig loads the config file, falls back to zero-config defaults
// when the file is absent, validates it structurally, and resolves it into a
// ResolvedConfig. This is the entry point for the normal runtime path.
func LoadRuntimeConfig(path string) (ResolvedConfig, error) {
	cfg, exists, err := LoadConfig(path)
	if err != nil {
		return ResolvedConfig{}, err
	}
	if !exists {
		var ok bool
		cfg, ok = ZeroConfigDefaults()
		if !ok {
			return ResolvedConfig{}, fmt.Errorf("no configuration found — run `tw config` to set up")
		}
	}
	if err := ValidateForRuntime(cfg); err != nil {
		return ResolvedConfig{}, err
	}
	return Resolve(cfg)
}

// ZeroConfigDefaults builds a minimal FileConfig from well-known API key
// environment variables when no config file exists. It returns the first
// provider whose env var is set, or (FileConfig{}, false) if none match.
// Ollama is excluded — it needs no API key and cannot be auto-detected.
func ZeroConfigDefaults() (FileConfig, bool) {
	for _, entry := range zeroConfigOrder {
		if os.Getenv(entry.envVar) == "" {
			continue
		}
		return FileConfig{
			SelectedProvider: entry.provider,
			Providers: map[string]ProviderConfig{
				entry.provider: {
					Auth:   "env",
					EnvVar: entry.envVar,
					Model:  models.DefaultModel(entry.provider),
				},
			},
		}, true
	}
	return FileConfig{}, false
}

// zeroConfigOrder defines the priority for auto-detecting a provider from
// environment variables. The first provider with its env var set wins.
var zeroConfigOrder = []struct {
	provider string
	envVar   string
}{
	{"anthropic", "ANTHROPIC_API_KEY"},
	{"openai", "OPENAI_API_KEY"},
	{"groq", "GROQ_API_KEY"},
	{"deepseek", "DEEPSEEK_API_KEY"},
	{"mistral", "MISTRAL_API_KEY"},
}

// NewClientFromResolved constructs a provider client from a ResolvedConfig.
// Auth credentials must already be resolved in rc.
func NewClientFromResolved(rc ResolvedConfig) (provider.AgentClient, error) {
	pcfg := provider.Config{
		APIKey:  rc.APIKey,
		Model:   rc.Model,
		BaseURL: rc.BaseURL,
	}
	switch rc.ProviderName {
	case "anthropic":
		return anthropic.New(pcfg)
	default:
		defaultURL, ok := openaicompat.DefaultBaseURL(rc.ProviderName)
		if !ok {
			if rc.BaseURL == "" {
				return nil, fmt.Errorf(
					"unknown provider %q — set base_url to use a custom OpenAI-compatible endpoint",
					rc.ProviderName,
				)
			}
			defaultURL = rc.BaseURL
		}
		return openaicompat.New(rc.ProviderName, defaultURL, pcfg)
	}
}

// ResolveAuth returns the resolved API key and base URL for a provider config.
// If the configured auth method fails, it tries the provider's well-known env
// var as a fallback before returning an error.
func ResolveAuth(name string, pc ProviderConfig) (ResolvedAuth, error) {
	key, err := resolveKey(name, pc)
	if err != nil {
		if fb := fallbackEnvVar(name); fb != "" {
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
			if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
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

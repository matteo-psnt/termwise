package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/zalando/go-keyring"

	"github.com/matteo-psnt/termwise/internal/cliname"
)

// ResolvedAuth holds the resolved API key and an optional base URL override.
type ResolvedAuth struct {
	APIKey  string
	BaseURL string
	Source  string // see ResolvedConfig.KeySource
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
	method := pc.AuthMethod()
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
		key, err := keyring.Get(keychainService, pc.KeychainEntry)
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

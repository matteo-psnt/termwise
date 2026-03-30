package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/zalando/go-keyring"
)

// DefaultKeychainEntry returns the keychain entry name used for a provider.
func DefaultKeychainEntry(provider string) string {
	return "termwise-" + provider
}

// StoreKeychain writes an API key into the macOS Keychain under the termwise service.
func StoreKeychain(provider, apiKey string) error {
	return keyring.Set("termwise", DefaultKeychainEntry(provider), apiKey)
}

// ResolvedAuth holds the resolved API key and the base URL (if any) for a provider.
type ResolvedAuth struct {
	APIKey  string
	BaseURL string // empty means use provider default
}

// ResolveAuth resolves the API key for a provider from its configured auth method.
// It also applies the auth fallback: if the configured method fails, it checks
// ANTHROPIC_API_KEY or OPENAI_API_KEY as a last resort before erroring.
func ResolveAuth(name string, pc ProviderConfig) (ResolvedAuth, error) {
	key, err := resolveKey(name, pc)
	if err != nil {
		// Fallback: try well-known env vars for the provider before giving up.
		if fallback := fallbackEnvVar(name); fallback != "" {
			if v := os.Getenv(fallback); v != "" {
				return ResolvedAuth{APIKey: v, BaseURL: pc.BaseURL}, nil
			}
		}
		return ResolvedAuth{}, err
	}
	return ResolvedAuth{APIKey: key, BaseURL: pc.BaseURL}, nil
}

func resolveKey(name string, pc ProviderConfig) (string, error) {
	method := pc.AuthMethod
	if method == "" {
		method = "env" // default
	}

	switch method {
	case "env":
		envVar := pc.EnvVar
		if envVar == "" {
			envVar = DefaultEnvVar(name)
		}
		if envVar == "" {
			// Provider has no known env var (e.g. ollama needs no key).
			return "", nil
		}
		val := os.Getenv(envVar)
		if val == "" {
			return "", fmt.Errorf("environment variable %q is not set", envVar)
		}
		return val, nil

	case "cmd":
		if pc.APIKeyCmd == "" {
			return "", fmt.Errorf("auth_method is \"cmd\" but api_key_cmd is not set in [providers.%s]", name)
		}
		out, err := exec.Command("sh", "-c", pc.APIKeyCmd).Output()
		if err != nil {
			// Pass through the command's stderr if available.
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
		entry := pc.KeychainEntry
		if entry == "" {
			return "", fmt.Errorf("auth_method is \"keychain\" but keychain_entry is not set in [providers.%s]", name)
		}
		key, err := keyring.Get("termwise", entry)
		if err != nil {
			return "", fmt.Errorf("keychain entry %q not found", entry)
		}
		return key, nil

	default:
		return "", fmt.Errorf(
			"unknown auth_method %q in [providers.%s]\n  Valid methods: env, cmd, keychain",
			method, name,
		)
	}
}

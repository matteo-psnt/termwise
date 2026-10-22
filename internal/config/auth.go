package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/matteo-psnt/termwise/internal/cliname"
)

// ResolvedAuth holds the resolved API key and an optional base URL override.
type ResolvedAuth struct {
	APIKey  string
	BaseURL string
	Source  string // see ResolvedConfig.KeySource
	// FallbackErr is set when the configured method failed and the key came
	// from the provider's default env var instead. The run still works, but
	// the config does not do what it says, so reports should show this.
	FallbackErr error
}

// apiKeyCmdTimeout bounds api_key_cmd. It is long enough for a biometric or
// master-password unlock prompt. A var so tests can shorten it.
var apiKeyCmdTimeout = 30 * time.Second

// ResolveAuth returns the resolved API key and base URL for a provider config.
// If the configured auth method fails, it tries the provider's well-known env
// var as a fallback before returning an error.
func ResolveAuth(name string, pc ProviderConfig) (ResolvedAuth, error) {
	key, source, err := resolveKey(name, pc)
	if err != nil {
		if fb := DefaultEnvVar(name); fb != "" {
			if v := os.Getenv(fb); v != "" {
				return ResolvedAuth{APIKey: v, BaseURL: pc.BaseURL, Source: "$" + fb, FallbackErr: err}, nil
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
		// A password manager waiting to be unlocked, or for input it will
		// never get, would otherwise hang every entry point. Stdin is left
		// unset, so the command reads /dev/null rather than the terminal.
		ctx, cancel := context.WithTimeout(context.Background(), apiKeyCmdTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sh", "-c", pc.APIKeyCmd)
		// Killing sh does not kill what it started, which can hold the
		// output pipe open; stop waiting for it shortly after.
		cmd.WaitDelay = time.Second
		out, err := cmd.Output()
		if ctx.Err() == context.DeadlineExceeded {
			return "", "", fmt.Errorf("api_key_cmd timed out after %s", apiKeyCmdTimeout)
		}
		// A background child the command started (`echo key; sleep 3 &`)
		// can hold stdout open after the command itself has exited 0. That
		// trips WaitDelay, but the key it printed is complete.
		if errors.Is(err, exec.ErrWaitDelay) && ctx.Err() == nil &&
			cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 0 && len(bytes.TrimSpace(out)) > 0 {
			err = nil
		}
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
				return "", "", fmt.Errorf("api_key_cmd failed: %s", firstLine(string(exitErr.Stderr), 200))
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

// firstLine returns the first non-blank line of s, cut to at most n runes.
// A password manager's stderr can run long or echo more than an error
// message, and the result ends up in terminal output and bug reports.
func firstLine(s string, n int) string {
	for line := range strings.Lines(s) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > n {
			return string(r[:n]) + "…"
		}
		return line
	}
	return ""
}

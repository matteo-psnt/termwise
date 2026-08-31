package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// DefaultConfigPath returns ~/.config/termwise/config.toml.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "termwise", "config.toml"), nil
}

// LoadConfig reads and parses the config file at path.
// Returns (cfg, false, nil) when the file does not exist — any other I/O or
// parse failure is returned as an error. Unknown keys are silently ignored;
// use CheckConfigWarnings to surface them at startup.
func LoadConfig(path string) (FileConfig, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return FileConfig{}, false, nil
		}
		return FileConfig{}, false, fmt.Errorf("reading config: %w", err)
	}

	var cfg FileConfig
	if err := toml.NewDecoder(bytes.NewReader(data)).Decode(&cfg); err != nil {
		return FileConfig{}, true, fmt.Errorf("config file is invalid: %w\n  File: %s", err, path)
	}
	return cfg, true, nil
}

// CheckConfigWarnings returns human-readable warnings about the config file
// at path — currently, one entry per unknown key. Returns nil when the file
// does not exist, cannot be read, or contains only known keys. Intended to be
// called once at startup so stale keys (e.g. from older versions of termwise)
// surface to the user without blocking the command.
func CheckConfigWarnings(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var probe FileConfig
	err = dec.Decode(&probe)
	if err == nil {
		return nil
	}
	var sm *toml.StrictMissingError
	if !errors.As(err, &sm) {
		return nil
	}
	displayPath := shortenHomePath(path)
	out := make([]string, 0, len(sm.Errors))
	for _, de := range sm.Errors {
		key := strings.Join(de.Key(), ".")
		line, _ := de.Position()
		out = append(out, fmt.Sprintf("%s:%d: unknown key %q (ignored; remove to silence)", displayPath, line, key))
	}
	return out
}

// shortenHomePath replaces the leading $HOME segment with "~" for compactness
// in user-facing messages. Returns path unchanged when not under $HOME.
func shortenHomePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rel, ok := strings.CutPrefix(path, home+string(os.PathSeparator)); ok {
		return "~" + string(os.PathSeparator) + rel
	}
	return path
}

// SaveConfig writes cfg to path atomically.
// The config directory is created (mode 0700) if it does not exist, and the
// config file is written with mode 0600 before being renamed into place.
func SaveConfig(path string, cfg FileConfig) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return atomicWriteFile(path, data)
}

// atomicWriteFile writes data to path atomically: it creates a temp file in the
// same directory, syncs it, chmods it to 0600, then renames it into place.
// The directory is created with mode 0700 if it does not exist.
func atomicWriteFile(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err == nil {
			return
		}
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("syncing %s: %w", path, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err = os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err = os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("saving %s: %w", path, err)
	}
	return nil
}

package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

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
// parse failure is returned as an error.
func LoadConfig(path string) (FileConfig, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return FileConfig{}, false, nil
		}
		return FileConfig{}, false, fmt.Errorf("reading config: %w", err)
	}

	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var cfg FileConfig
	if err := dec.Decode(&cfg); err != nil {
		return FileConfig{}, true, fmt.Errorf("config file is invalid: %w\n  File: %s", err, path)
	}
	return cfg, true, nil
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

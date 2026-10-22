package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/matteo-psnt/termwise/internal/cliname"
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

// ConfigExists reports whether there is a config file at path. A file that
// exists but cannot be parsed still counts: it is the user's, and setup
// should not act as if it were absent.
func ConfigExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
	displayPath := ShortenHome(path)
	out := make([]string, 0, len(sm.Errors))
	for _, de := range sm.Errors {
		key := strings.Join(de.Key(), ".")
		line, _ := de.Position()
		out = append(out, fmt.Sprintf("%s:%d: unknown key %q (ignored; remove to silence)", displayPath, line, key))
	}
	return out
}

// ShortenHome replaces a leading $HOME with "~" for compactness in
// user-facing messages, and returns path unchanged when it is not under
// $HOME. It compares cleaned paths, so a trailing slash on $HOME or a
// "dir/../" in path does not defeat it.
func ShortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.Join("~", rel)
}

// saveWarnings is where SaveConfig reports a fallback rewrite; tests swap it.
var saveWarnings io.Writer = os.Stderr

// SaveConfig writes cfg to path atomically.
// The config directory is created (mode 0700) if it does not exist, and the
// config file is written with mode 0600 before being renamed into place.
// It holds the config lock while it reads and replaces the file; to change a
// config loaded from disk, use MutateConfig so the load is covered too.
//
// An existing file is edited rather than replaced, so the user's comments,
// formatting and unknown keys survive a save from the CLI or the setup
// screen. When it cannot be edited safely it is rewritten from scratch, after
// a copy of it is kept at path.bak and a warning is printed.
func SaveConfig(path string, cfg FileConfig) error {
	unlock, err := lockConfig(path)
	if err != nil {
		return err
	}
	defer unlock()
	return saveConfig(path, cfg)
}

// saveConfig is SaveConfig for a caller that already holds the config lock.
func saveConfig(path string, cfg FileConfig) error {
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading config: %w", err)
	}
	if err != nil {
		data, err := toml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}
		return atomicWriteFile(path, data)
	}

	data, spliceErr := spliceConfig(old, cfg)
	if spliceErr == nil {
		return atomicWriteFile(path, data)
	}
	data, err = toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	backup := path + ".bak"
	if err := atomicWriteFile(backup, old); err != nil {
		return fmt.Errorf("backing up config before rewriting it: %w", err)
	}
	_, _ = fmt.Fprintf(saveWarnings, "%s: warning: could not edit %s in place (%v); rewrote it, previous version saved to %s\n",
		cliname.Name(), ShortenHome(path), spliceErr, ShortenHome(backup))
	return atomicWriteFile(path, data)
}

// atomicWriteFile writes data to path atomically: it creates a temp file in the
// same directory, syncs it, chmods it to 0600, then renames it into place.
// The directory is created with mode 0700 if it does not exist.
func atomicWriteFile(path string, data []byte) (err error) {
	// A config kept in a dotfiles repo is often a symlink to it. Renaming
	// over the link would replace it with a plain file and silently fork
	// the config from the repo, so write to what it points at instead.
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
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

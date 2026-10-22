package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// lockTimeout bounds how long a save waits for another termwise process
// (the setup screen, a second CLI call) to finish its own. Saves take
// milliseconds, so running into it means something is stuck.
var lockTimeout = 3 * time.Second

// ErrConfigLocked is returned when another process holds the config lock
// for longer than lockTimeout.
var ErrConfigLocked = errors.New("the config file is being saved by another termwise process")

// lockConfig takes an advisory lock that serializes read-modify-write cycles
// on the config at path, and returns the function that releases it. The lock
// is on a side file: the config itself is replaced by rename on every save,
// so a lock on its inode would not be seen by the next process.
//
// The side file sits next to path as given, not next to a symlink's target:
// a config.toml linked into a dotfiles repo would otherwise leave an
// untracked config.toml.lock in the repo. Every termwise process uses the
// same default path (~/.config/termwise/config.toml), so they still contend
// on one lock file.
func lockConfig(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating directory: %w", err)
	}
	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}
	deadline := time.Now().Add(lockTimeout)
	for {
		ok, err := tryLockFile(f)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("locking %s: %w", lockPath, err)
		}
		if ok {
			return func() {
				unlockFile(f)
				_ = f.Close()
			}, nil
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("%w — try again in a moment (lock: %s)", ErrConfigLocked, ShortenHome(lockPath))
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// LoadError marks an error from loading the config in MutateConfig, as
// opposed to one returned by its callback.
type LoadError struct{ Err error }

func (e *LoadError) Error() string { return e.Err.Error() }
func (e *LoadError) Unwrap() error { return e.Err }

// MutateConfig loads the config at path, applies fn and saves the result,
// holding the config lock throughout so a concurrent save cannot land
// between the load and the save and be silently overwritten. A missing file
// starts from an empty config; a file that fails to load is returned as a
// *LoadError.
func MutateConfig(path string, fn func(*FileConfig) error) error {
	unlock, err := lockConfig(path)
	if err != nil {
		return err
	}
	defer unlock()
	cfg, _, err := LoadConfig(path)
	if err != nil {
		return &LoadError{Err: err}
	}
	if err := fn(&cfg); err != nil {
		return err
	}
	return saveConfig(path, cfg)
}

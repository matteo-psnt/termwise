//go:build unix

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestMutateConfigSerializesConcurrentSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := SaveConfig(path, FileConfig{SelectedProvider: "openai"}); err != nil {
		t.Fatal(err)
	}
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- MutateConfig(path, func(cfg *FileConfig) error {
				SetProvider(cfg, fmt.Sprintf("p%d", i), ProviderConfig{Model: "m"})
				return nil
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg, _, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != n {
		t.Errorf("%d of %d concurrent changes survived", len(cfg.Providers), n)
	}
}

func TestSaveConfigTimesOutOnHeldLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	unlock, err := lockConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	old := lockTimeout
	lockTimeout = 100 * time.Millisecond
	t.Cleanup(func() { lockTimeout = old })

	err = SaveConfig(path, FileConfig{SelectedProvider: "openai"})
	if !errors.Is(err, ErrConfigLocked) {
		t.Errorf("SaveConfig with the lock held = %v, want ErrConfigLocked", err)
	}
}

func TestLockFileSitsNextToSymlinkNotTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`selected_provider = "openai"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "termwise", "config.toml")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := MutateConfig(link, func(c *FileConfig) error { c.SelectedProvider = "anthropic"; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target + ".lock"); err == nil {
		t.Error("lock file created in the dotfiles directory, next to the symlink's target")
	}
	if _, err := os.Stat(link + ".lock"); err != nil {
		t.Errorf("no lock file next to the symlink: %v", err)
	}
}

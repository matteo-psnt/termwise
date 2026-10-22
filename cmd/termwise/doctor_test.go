package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
)

// isolateConfigEnv points HOME at a temp dir and clears every variable that
// can select a provider, so a test sees only what it sets itself. It returns
// the default config path under the new HOME.
func isolateConfigEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(config.EnvProvider, "")
	t.Setenv(config.EnvModel, "")
	for _, p := range provider.Catalog() {
		if p.DefaultEnvVar != "" {
			t.Setenv(p.DefaultEnvVar, "")
		}
	}
	return filepath.Join(home, ".config", "termwise", "config.toml")
}

// captureStdout returns what fn printed to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	return <-done
}

func TestDoctorAppliesEnvOverrides(t *testing.T) {
	cfgPath := isolateConfigEnv(t)
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvModel, "qwen3")

	var ok bool
	out := captureStdout(t, func() { ok = runDoctor(cfgPath, false) })
	if !ok {
		t.Errorf("doctor failed with only %s set, which the runtime accepts:\n%s", config.EnvProvider, out)
	}
	for _, want := range []string{"Override: " + config.EnvProvider + "=ollama", "Model: qwen3"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output does not contain %q:\n%s", want, out)
		}
	}
}

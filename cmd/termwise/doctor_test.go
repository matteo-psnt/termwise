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
	return captureFile(t, &os.Stdout, fn)
}

// captureFile returns what fn wrote to *f, which it swaps for a pipe.
func captureFile(t *testing.T, f **os.File, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := *f
	*f = w
	defer func() { *f = orig }()

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

func TestDoctorFlagsAuthFallback(t *testing.T) {
	cfgPath := isolateConfigEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
	writeTestConfig(t, cfgPath, `selected_provider = "anthropic"

[providers.anthropic]
auth        = "cmd"
api_key_cmd = "exit 1"
model       = "claude-haiku-4-5-20251001"
`)

	out := captureStdout(t, func() { runDoctor(cfgPath, false) })
	if !strings.Contains(out, "! Auth: cmd (exit 1) failed, using $ANTHROPIC_API_KEY instead") {
		t.Errorf("doctor does not flag the fallback to $ANTHROPIC_API_KEY:\n%s", out)
	}
	if strings.Contains(out, "key resolved") {
		t.Errorf("doctor reports the configured method as working:\n%s", out)
	}
}

func writeTestConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorWarnsAboutInvalidEffort(t *testing.T) {
	cfgPath := isolateConfigEnv(t)
	writeTestConfig(t, cfgPath, `selected_provider = "ollama"

[providers.ollama]
model  = "qwen3"
effort = "hihg"
`)
	var ok bool
	out := captureStdout(t, func() { ok = runDoctor(cfgPath, false) })
	if !strings.Contains(out, `! Effort: "hihg" is not a valid level`) || !strings.Contains(out, "xhigh, max") {
		t.Errorf("doctor does not flag the bad effort:\n%s", out)
	}
	if !ok {
		t.Errorf("an ignored effort failed doctor:\n%s", out)
	}
}

func TestDoctorModelHintWithoutFile(t *testing.T) {
	cfgPath := isolateConfigEnv(t)
	t.Setenv(config.EnvProvider, "ollama")

	out := captureStdout(t, func() { runDoctor(cfgPath, false) })
	if !strings.Contains(out, "Model: not set") {
		t.Fatalf("doctor output does not report the missing model:\n%s", out)
	}
	if strings.Contains(out, "config model set") {
		t.Errorf("doctor suggests `config model set`, which fails with no file:\n%s", out)
	}
	for _, want := range []string{config.EnvModel + "=<model>", "config use ollama --model <model>"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output does not contain %q:\n%s", want, out)
		}
	}
}

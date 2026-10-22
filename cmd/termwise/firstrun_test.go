package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matteo-psnt/termwise/internal/cliname"
)

func setupShellHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")
	t.Setenv(cliname.EnvVar, "")
	return filepath.Join(home, ".zshrc")
}

func TestOfferShellIntegrationDefaultsToYes(t *testing.T) {
	rc := setupShellHome(t)
	var out bytes.Buffer
	offerShellIntegration(strings.NewReader("\n"), &out)

	data, err := os.ReadFile(rc)
	if err != nil {
		t.Fatalf("rc file not written: %v", err)
	}
	if !strings.Contains(string(data), `eval "$(termwise init zsh)"`) {
		t.Errorf("rc file = %q, want the init line", data)
	}
}

func TestOfferShellIntegrationRespectsNo(t *testing.T) {
	rc := setupShellHome(t)
	var out bytes.Buffer
	offerShellIntegration(strings.NewReader("n\n"), &out)

	if _, err := os.Stat(rc); !os.IsNotExist(err) {
		t.Errorf("rc file touched after answering no")
	}
	if !strings.Contains(out.String(), "termwise init --install") {
		t.Errorf("output %q does not say how to add it later", out.String())
	}
}

func TestOfferShellIntegrationSilentWhenLoaded(t *testing.T) {
	setupShellHome(t)
	t.Setenv(cliname.EnvVar, "tw")
	var out bytes.Buffer
	offerShellIntegration(strings.NewReader("\n"), &out)
	if out.Len() != 0 {
		t.Errorf("printed %q with the integration already loaded", out.String())
	}
}

func TestOfferShellIntegrationTreatsEOFAsNo(t *testing.T) {
	rc := setupShellHome(t)
	var out bytes.Buffer
	offerShellIntegration(strings.NewReader(""), &out)

	if _, err := os.Stat(rc); !os.IsNotExist(err) {
		t.Errorf("rc file written after Ctrl+D at the prompt")
	}
}

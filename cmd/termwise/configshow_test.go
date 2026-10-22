package main

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/matteo-psnt/termwise/internal/config"
)

// With no file the runtime runs from an API key in the environment, so
// `show --json` has to print that config: a script parsing stdout would
// otherwise get nothing on a successful exit.
func TestShowJSONDescribesEnvOnlyConfig(t *testing.T) {
	isolateConfigEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
	if err := configShowCmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = configShowCmd.Flags().Set("json", "false") })

	var runErr error
	out := captureStdout(t, func() { runErr = configShowCmd.RunE(configShowCmd, nil) })
	if runErr != nil {
		t.Fatal(runErr)
	}
	var got showJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if got.Source != sourceEnvironment || got.SelectedProvider != "anthropic" {
		t.Errorf("source = %q, selected_provider = %q; want environment, anthropic", got.Source, got.SelectedProvider)
	}
	if auth := got.Providers["anthropic"].Auth; auth.Source != "ANTHROPIC_API_KEY" {
		t.Errorf("auth source = %q, want ANTHROPIC_API_KEY", auth.Source)
	}
}

func TestAuthSummaryTruncatesOnRuneBoundary(t *testing.T) {
	// Two-byte runes throughout, so any odd byte offset splits one.
	cmd := "pass show " + strings.Repeat("ключ", 10)
	got := authSummary("openai", config.ProviderConfig{Auth: "cmd", APIKeyCmd: cmd})
	if !utf8.ValidString(got) {
		t.Errorf("authSummary = %q, not valid UTF-8", got)
	}
	if !strings.HasSuffix(got, "...)") {
		t.Errorf("authSummary = %q, want it truncated", got)
	}
}

// show reports what the runtime would use, so TERMWISE_PROVIDER and
// TERMWISE_MODEL must be applied and marked.
func TestShowAppliesAndMarksEnvOverrides(t *testing.T) {
	cfgPath := isolateConfigEnv(t)
	if err := config.SaveConfig(cfgPath, config.FileConfig{
		SelectedProvider: "anthropic",
		Providers:        map[string]config.ProviderConfig{"anthropic": {Model: "claude-x"}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.EnvProvider, "openai")
	t.Setenv(config.EnvModel, "gpt-x")

	if err := configShowCmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	var runErr error
	out := captureStdout(t, func() { runErr = configShowCmd.RunE(configShowCmd, nil) })
	_ = configShowCmd.Flags().Set("json", "false")
	if runErr != nil {
		t.Fatal(runErr)
	}
	var got showJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if got.SelectedProvider != "openai" || got.Providers["openai"].Model != "gpt-x" {
		t.Errorf("selected = %q, model = %q; want openai, gpt-x", got.SelectedProvider, got.Providers["openai"].Model)
	}
	if got.Overrides[config.EnvProvider] != "openai" || got.Overrides[config.EnvModel] != "gpt-x" {
		t.Errorf("overrides = %v", got.Overrides)
	}

	out = captureStdout(t, func() { runErr = configShowCmd.RunE(configShowCmd, nil) })
	if runErr != nil {
		t.Fatal(runErr)
	}
	if !strings.Contains(out, "* openai (active) ["+config.EnvProvider+"]") || !strings.Contains(out, "gpt-x ["+config.EnvModel+"]") {
		t.Errorf("show output does not mark the overrides:\n%s", out)
	}
}

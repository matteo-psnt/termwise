package main

import (
	"testing"

	"github.com/matteo-psnt/termwise/internal/config"
)

// `config model list` and `config test` have to talk to the provider the
// runtime would use, or they report "no provider selected" for a setup that
// `tw` runs fine.
func TestEffectiveProviderConfigMatchesRuntime(t *testing.T) {
	cfgPath := isolateConfigEnv(t)

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
	name, pc, err := effectiveProviderConfig("")
	if err != nil || name != "anthropic" || pc.EnvVar != "ANTHROPIC_API_KEY" {
		t.Errorf("env-only: got %q, %+v, %v; want anthropic via $ANTHROPIC_API_KEY", name, pc, err)
	}

	writeTestConfig(t, cfgPath, `selected_provider = "anthropic"

[providers.anthropic]
model = "claude-haiku-4-5-20251001"
`)
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvModel, "qwen3")
	name, pc, err = effectiveProviderConfig("")
	if err != nil || name != "ollama" || pc.Model != "qwen3" {
		t.Errorf("with overrides: got %q, %+v, %v; want ollama with qwen3", name, pc, err)
	}
}

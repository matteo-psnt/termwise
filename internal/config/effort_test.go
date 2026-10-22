package config

import (
	"slices"
	"strings"
	"testing"

	"github.com/matteo-psnt/termwise/internal/models"
)

func TestEffortLevelsIncludeXhighAndMax(t *testing.T) {
	want := []string{"low", "medium", "high", "xhigh", "max"}
	if !slices.Equal(EffortLevels, want) {
		t.Errorf("EffortLevels = %v, want %v", EffortLevels, want)
	}
}

// A typo must not reach the API: it is treated as unset.
func TestEffectiveEffortIgnoresInvalidValue(t *testing.T) {
	const prov, model = "anthropic", "claude-sonnet-4-6"
	if !models.SupportsThinking(prov, model) {
		t.Skip("catalog no longer marks the model as thinking-capable")
	}
	if got := EffectiveEffort(prov, model, "hihg"); got != DefaultEffort {
		t.Errorf("EffectiveEffort(hihg) = %q, want %q", got, DefaultEffort)
	}
	if got := EffectiveEffort(prov, model, "max"); got != "max" {
		t.Errorf("EffectiveEffort(max) = %q, want max", got)
	}
}

func TestCheckConfigWarningsReportsInvalidEffort(t *testing.T) {
	path := writeConfig(t, `selected_provider = "anthropic"

[providers.anthropic]
model = "claude-sonnet-4-6"
effort = "hihg"

[providers.openai]
effort = "high"`)
	warnings := CheckConfigWarnings(path)
	if len(warnings) != 1 || !strings.Contains(warnings[0], `"hihg"`) || !strings.Contains(warnings[0], "providers.anthropic") {
		t.Errorf("warnings = %q, want one naming the bad effort", warnings)
	}
	// Not fatal: the config still validates for the runtime.
	cfg, _, _ := LoadConfig(path)
	for _, e := range Validate(cfg) {
		if strings.Contains(e, "effort") {
			t.Errorf("Validate reports effort as an error: %s", e)
		}
	}
}

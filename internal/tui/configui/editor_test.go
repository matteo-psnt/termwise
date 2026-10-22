package configui

import (
	"testing"

	"github.com/matteo-psnt/termwise/internal/config"
)

func TestDisplayValueFormatsToggleInTitleCase(t *testing.T) {
	boolPtr := func(v bool) *bool { return &v }
	cfg := config.FileConfig{
		Settings: config.SettingsConfig{
			LLMJudge:   boolPtr(true),
			AutoResume: boolPtr(false),
		},
	}

	if got := displayValue(mustSetting(t, "llm_judge"), cfg); got != "On" {
		t.Fatalf("displayValue(llm_judge) = %q, want %q", got, "On")
	}
	if got := displayValue(mustSetting(t, "auto_resume"), cfg); got != "Off" {
		t.Fatalf("displayValue(auto_resume) = %q, want %q", got, "Off")
	}
}

func mustSetting(t *testing.T, key string) config.SettingDef {
	t.Helper()
	def, ok := config.FindSetting(key)
	if !ok {
		t.Fatalf("no setting %q", key)
	}
	return def
}

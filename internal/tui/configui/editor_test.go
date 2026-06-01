package configui

import (
	"testing"

	"github.com/matteo-psnt/termwise/internal/config"
)

func TestDisplayValueFormatsToggleInTitleCase(t *testing.T) {
	cfg := config.FileConfig{
		Settings: config.SettingsConfig{
			LLMJudge:   true,
			AutoResume: false,
		},
	}

	llmJudge := config.Settings[2]
	if got := displayValue(llmJudge, cfg); got != "On" {
		t.Fatalf("displayValue(llm_judge) = %q, want %q", got, "On")
	}

	autoResume := config.Settings[3]
	if got := displayValue(autoResume, cfg); got != "Off" {
		t.Fatalf("displayValue(auto_resume) = %q, want %q", got, "Off")
	}
}

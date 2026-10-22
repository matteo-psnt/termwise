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

	llmJudge := config.Settings[2]
	if got := displayValue(llmJudge, cfg); got != "On" {
		t.Fatalf("displayValue(llm_judge) = %q, want %q", got, "On")
	}

	autoResume := config.Settings[3]
	if got := displayValue(autoResume, cfg); got != "Off" {
		t.Fatalf("displayValue(auto_resume) = %q, want %q", got, "Off")
	}
}

func TestListWindow(t *testing.T) {
	tests := []struct {
		n, cursor, size, start, end int
	}{
		{3, 0, 10, 0, 3},
		{50, 0, 10, 0, 10},
		{50, 4, 10, 0, 10},
		{50, 20, 10, 15, 25},
		{50, 49, 10, 40, 50},
	}
	for _, tt := range tests {
		start, end := listWindow(tt.n, tt.cursor, tt.size)
		if start != tt.start || end != tt.end {
			t.Errorf("listWindow(%d, %d, %d) = [%d, %d), want [%d, %d)", tt.n, tt.cursor, tt.size, start, end, tt.start, tt.end)
		}
		if tt.cursor < start || tt.cursor >= end {
			t.Errorf("listWindow(%d, %d, %d): cursor outside window", tt.n, tt.cursor, tt.size)
		}
	}
}

package configui

import (
	"strings"
	"testing"
	"unicode/utf8"

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

func TestDescribeAuthTruncatesByRune(t *testing.T) {
	cmd := "op read op://" + strings.Repeat("é", 40)
	got := describeAuth("openai", config.ProviderConfig{Auth: "cmd", APIKeyCmd: cmd})
	if !utf8.ValidString(got) {
		t.Errorf("describeAuth split a character: %q", got)
	}
	if want := "cmd · " + string([]rune(cmd)[:37]) + "..."; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

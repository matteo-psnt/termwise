package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigParsesToolsBashAndTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := `
active_provider = "anthropic"

[providers.anthropic]
auth_method = "env"
env_var = "ANTHROPIC_API_KEY"
model = "claude-haiku-4-5-20251001"

[shell]
keybinding = "^T"

[tools.bash]
allow = ["git diff", "git log", "rg"]

[tui]
theme = "ocean"
show_footer = true
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, exists, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if !exists {
		t.Fatalf("expected config to exist")
	}
	if got := cfg.Tools.Bash.Allow; len(got) != 3 || got[0] != "git diff" || got[2] != "rg" {
		t.Fatalf("unexpected bash allow rules: %#v", got)
	}
	if cfg.TUI.Theme != "ocean" {
		t.Fatalf("expected theme ocean, got %q", cfg.TUI.Theme)
	}
	if cfg.Shell.Keybinding != "^T" {
		t.Fatalf("expected keybinding ^T, got %q", cfg.Shell.Keybinding)
	}
}

func TestLoadConfigMigratesLegacyShellAllow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := `
active_provider = "anthropic"

[providers.anthropic]
model = "claude-haiku-4-5-20251001"

[shell]
allow = ["git diff", "rg"]
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, exists, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if !exists {
		t.Fatalf("expected config to exist")
	}
	if got := cfg.Tools.Bash.Allow; len(got) != 2 || got[0] != "git diff" || got[1] != "rg" {
		t.Fatalf("expected migrated allow rules, got %#v", got)
	}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if strings.Contains(string(saved), "allow = [\"git diff\", \"rg\"]") && !strings.Contains(string(saved), "[tools.bash]") {
		t.Fatalf("expected migrated allow rules to be written under [tools.bash], got %s", saved)
	}
	if strings.Contains(string(saved), "[shell]\nallow") {
		t.Fatalf("expected legacy allow to be removed after save, got %s", saved)
	}
}

func TestValidateRejectsUnknownTheme(t *testing.T) {
	cfg := Config{
		ActiveProvider: "anthropic",
		Providers: map[string]ProviderConfig{
			"anthropic": {Model: "claude-haiku-4-5-20251001"},
		},
		TUI: TUIConfig{Theme: "neon"},
	}

	errs := Validate(cfg)
	if len(errs) == 0 {
		t.Fatalf("expected validation errors")
	}
	if !strings.Contains(errs[0], "unknown tui.theme") {
		t.Fatalf("expected theme validation error, got %v", errs)
	}
}

func TestSetAndGetThemeValue(t *testing.T) {
	var cfg Config

	if err := SetValue(&cfg, "tui.theme", "Ocean"); err != nil {
		t.Fatalf("SetValue returned error: %v", err)
	}
	got, err := GetValue(cfg, "tui.theme")
	if err != nil {
		t.Fatalf("GetValue returned error: %v", err)
	}
	if got != "ocean" {
		t.Fatalf("expected normalized theme name, got %q", got)
	}
}

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Load / Save
// ---------------------------------------------------------------------------

func TestLoadConfigParsesNewFormat(t *testing.T) {
	path := writeConfig(t, `
selected_provider = "anthropic"

[providers.anthropic]
auth     = "env"
env_var  = "ANTHROPIC_API_KEY"
model    = "claude-haiku-4-5-20251001"

[ui]
theme       = "ocean"
show_footer = true

[shell]
keybinding = "^T"

[policies.bash]
llm_judge = true
`)

	cfg, exists, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !exists {
		t.Fatal("expected config to exist")
	}

	assertEqual(t, "selected_provider", "anthropic", cfg.SelectedProvider)
	assertEqual(t, "providers.anthropic.auth", "env", cfg.Providers["anthropic"].Auth)
	assertEqual(t, "providers.anthropic.model", "claude-haiku-4-5-20251001", cfg.Providers["anthropic"].Model)
	assertEqual(t, "ui.theme", "ocean", cfg.UI.Theme)
	assertEqual(t, "shell.keybinding", "^T", cfg.Shell.Keybinding)

	if cfg.UI.ShowFooter == nil || !*cfg.UI.ShowFooter {
		t.Error("expected ui.show_footer to be true")
	}
	if !cfg.Policies.Bash.LLMJudge {
		t.Error("expected policies.bash.llm_judge to be true")
	}
}

func TestLoadConfigAbsentFileReturnsNotExists(t *testing.T) {
	_, exists, err := LoadConfig(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Fatal("expected exists=false for missing file")
	}
}

func TestLoadConfigRejectsUnknownFields(t *testing.T) {
	path := writeConfig(t, `selected_provider = "anthropic"
unknown_field = "oops"`)
	_, _, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestLoadConfigRejectsOldKeys(t *testing.T) {
	// active_provider is no longer a valid field.
	path := writeConfig(t, `active_provider = "anthropic"`)
	_, _, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error: old key active_provider should be rejected")
	}
}

func TestSaveAndLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	showFooter := true
	original := FileConfig{
		SelectedProvider: "openai",
		Providers: map[string]ProviderConfig{
			"openai": {Auth: "env", EnvVar: "OPENAI_API_KEY", Model: "gpt-4o"},
		},
		UI:    UIConfig{Theme: "ocean", ShowFooter: &showFooter},
		Shell: ShellConfig{Keybinding: "^T"},
		Policies: PoliciesConfig{
			Bash: BashPolicyConfig{LLMJudge: true},
		},
	}

	if err := SaveConfig(path, original); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, exists, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !exists {
		t.Fatal("expected config to exist after save")
	}

	assertEqual(t, "SelectedProvider", original.SelectedProvider, loaded.SelectedProvider)
	assertEqual(t, "providers.openai.model", original.Providers["openai"].Model, loaded.Providers["openai"].Model)
	assertEqual(t, "ui.theme", original.UI.Theme, loaded.UI.Theme)
	if loaded.UI.ShowFooter == nil || *loaded.UI.ShowFooter != showFooter {
		t.Errorf("ui.show_footer: want %v, got %v", showFooter, loaded.UI.ShowFooter)
	}
}

// ---------------------------------------------------------------------------
// Validate
// ---------------------------------------------------------------------------

func TestValidateRejectsUnknownTheme(t *testing.T) {
	cfg := FileConfig{
		SelectedProvider: "anthropic",
		Providers:        map[string]ProviderConfig{"anthropic": {Model: "claude-haiku-4-5-20251001"}},
		UI:               UIConfig{Theme: "neon"},
	}
	errs := Validate(cfg)
	if len(errs) == 0 {
		t.Fatal("expected validation error for unknown theme")
	}
	if !strings.Contains(errs[0], "unknown ui.theme") {
		t.Errorf("unexpected error: %q", errs[0])
	}
}

func TestValidateRejectsAbsentSelectedProvider(t *testing.T) {
	errs := Validate(FileConfig{})
	if len(errs) == 0 {
		t.Fatal("expected error when selected_provider is empty")
	}
	if !strings.Contains(errs[0], "selected_provider") {
		t.Errorf("unexpected error: %q", errs[0])
	}
}

func TestValidateRejectsMissingProviderBlock(t *testing.T) {
	cfg := FileConfig{SelectedProvider: "openai"} // no providers map
	errs := Validate(cfg)
	if len(errs) == 0 {
		t.Fatal("expected error for missing provider block")
	}
}

func TestValidateRejectsMissingModel(t *testing.T) {
	cfg := FileConfig{
		SelectedProvider: "anthropic",
		Providers:        map[string]ProviderConfig{"anthropic": {}}, // no model
	}
	errs := Validate(cfg)
	if len(errs) == 0 {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(errs[0], "missing model") {
		t.Errorf("unexpected error: %q", errs[0])
	}
}

func TestValidateRejectsUnknownAuthMethod(t *testing.T) {
	cfg := FileConfig{
		SelectedProvider: "anthropic",
		Providers: map[string]ProviderConfig{
			"anthropic": {Auth: "magic", Model: "claude-haiku-4-5-20251001"},
		},
	}
	errs := Validate(cfg)
	if len(errs) == 0 {
		t.Fatal("expected error for unknown auth method")
	}
	if !strings.Contains(errs[0], "unknown auth") {
		t.Errorf("unexpected error: %q", errs[0])
	}
}

func TestValidateAcceptsAllAuthMethods(t *testing.T) {
	for _, method := range []string{"env", "cmd", "keychain", ""} {
		cfg := FileConfig{
			SelectedProvider: "anthropic",
			Providers: map[string]ProviderConfig{
				"anthropic": {Auth: method, Model: "claude-haiku-4-5-20251001"},
			},
		}
		errs := Validate(cfg)
		// Theme check may not produce errors, but auth should be valid.
		for _, e := range errs {
			if strings.Contains(e, "unknown auth") {
				t.Errorf("auth method %q unexpectedly rejected: %s", method, e)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Resolve / auth
// ---------------------------------------------------------------------------

func TestResolveAuthEnv(t *testing.T) {
	t.Setenv("TEST_API_KEY_ABC", "sk-test")
	pc := ProviderConfig{Auth: "env", EnvVar: "TEST_API_KEY_ABC"}
	auth, err := ResolveAuth("openai", pc)
	if err != nil {
		t.Fatalf("ResolveAuth: %v", err)
	}
	assertEqual(t, "APIKey", "sk-test", auth.APIKey)
}

func TestResolveAuthEnvMissingVarReturnsError(t *testing.T) {
	// Use groq — it has no fallback env var, so a missing configured var
	// cannot be rescued and must produce an error.
	os.Unsetenv("TERMWISE_TEST_MISSING_VAR")
	os.Unsetenv("GROQ_API_KEY")
	pc := ProviderConfig{Auth: "env", EnvVar: "TERMWISE_TEST_MISSING_VAR"}
	_, err := ResolveAuth("groq", pc)
	if err == nil {
		t.Fatal("expected error for unset env var")
	}
}

func TestResolveAuthCmd(t *testing.T) {
	pc := ProviderConfig{Auth: "cmd", APIKeyCmd: "echo sk-from-cmd"}
	auth, err := ResolveAuth("openai", pc)
	if err != nil {
		t.Fatalf("ResolveAuth cmd: %v", err)
	}
	assertEqual(t, "APIKey", "sk-from-cmd", auth.APIKey)
}

func TestResolveAuthCmdFailureReturnsError(t *testing.T) {
	// Use groq — no fallback env var, so a failing cmd cannot be rescued.
	os.Unsetenv("GROQ_API_KEY")
	pc := ProviderConfig{Auth: "cmd", APIKeyCmd: "exit 1"}
	_, err := ResolveAuth("groq", pc)
	if err == nil {
		t.Fatal("expected error for failing cmd")
	}
}

func TestResolveAuthDefaultsToEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-default-env")
	// Auth field is empty — should default to env using the provider default var.
	pc := ProviderConfig{Model: "gpt-4o"}
	auth, err := ResolveAuth("openai", pc)
	if err != nil {
		t.Fatalf("ResolveAuth: %v", err)
	}
	assertEqual(t, "APIKey", "sk-default-env", auth.APIKey)
}

// ---------------------------------------------------------------------------
// ZeroConfigDefaults
// ---------------------------------------------------------------------------

func TestZeroConfigDefaultsPrefersAnthropic(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
	t.Setenv("OPENAI_API_KEY", "sk-oai")
	cfg, ok := ZeroConfigDefaults()
	if !ok {
		t.Fatal("expected ZeroConfigDefaults to find a provider")
	}
	assertEqual(t, "SelectedProvider", "anthropic", cfg.SelectedProvider)
}

func TestZeroConfigDefaultsNoneSetReturnsFalse(t *testing.T) {
	for _, v := range []string{
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GROQ_API_KEY",
		"DEEPSEEK_API_KEY", "MISTRAL_API_KEY",
	} {
		os.Unsetenv(v)
	}
	_, ok := ZeroConfigDefaults()
	if ok {
		t.Fatal("expected ZeroConfigDefaults to return false with no env vars set")
	}
}

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

func TestUseProviderSetsSelectedProvider(t *testing.T) {
	var cfg FileConfig
	if err := UseProvider(&cfg, "openai"); err != nil {
		t.Fatalf("UseProvider: %v", err)
	}
	assertEqual(t, "SelectedProvider", "openai", cfg.SelectedProvider)
}

func TestUseProviderRejectsUnknown(t *testing.T) {
	var cfg FileConfig
	if err := UseProvider(&cfg, "foobar"); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestSetModelCreatesProviderBlock(t *testing.T) {
	var cfg FileConfig
	if err := SetModel(&cfg, "anthropic", "claude-sonnet-4-6"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}
	if got := cfg.Providers["anthropic"].Model; got != "claude-sonnet-4-6" {
		t.Errorf("want claude-sonnet-4-6, got %q", got)
	}
}

func TestSetAuthEnvClearsPreviousAuthFields(t *testing.T) {
	cfg := FileConfig{
		Providers: map[string]ProviderConfig{
			"openai": {Auth: "keychain", KeychainEntry: "termwise-openai", Model: "gpt-4o"},
		},
	}
	if err := SetAuthEnv(&cfg, "openai", "OPENAI_API_KEY"); err != nil {
		t.Fatalf("SetAuthEnv: %v", err)
	}
	pc := cfg.Providers["openai"]
	assertEqual(t, "auth", "env", pc.Auth)
	assertEqual(t, "env_var", "OPENAI_API_KEY", pc.EnvVar)
	if pc.KeychainEntry != "" {
		t.Errorf("expected keychain_entry to be cleared, got %q", pc.KeychainEntry)
	}
	// Model must be preserved.
	assertEqual(t, "model", "gpt-4o", pc.Model)
}

func TestSetAuthCmdClearsPreviousAuthFields(t *testing.T) {
	cfg := FileConfig{
		Providers: map[string]ProviderConfig{
			"openai": {Auth: "env", EnvVar: "OPENAI_API_KEY"},
		},
	}
	if err := SetAuthCmd(&cfg, "openai", "op read op://vault/key"); err != nil {
		t.Fatalf("SetAuthCmd: %v", err)
	}
	pc := cfg.Providers["openai"]
	assertEqual(t, "auth", "cmd", pc.Auth)
	if pc.EnvVar != "" {
		t.Errorf("expected env_var to be cleared, got %q", pc.EnvVar)
	}
}

func TestRemoveProviderSucceeds(t *testing.T) {
	cfg := FileConfig{
		SelectedProvider: "anthropic",
		Providers: map[string]ProviderConfig{
			"anthropic": {Model: "claude-haiku-4-5-20251001"},
			"openai":    {Model: "gpt-4o"},
		},
	}
	if err := RemoveProvider(&cfg, "openai"); err != nil {
		t.Fatalf("RemoveProvider: %v", err)
	}
	if _, ok := cfg.Providers["openai"]; ok {
		t.Error("expected openai block to be removed")
	}
}

func TestRemoveActiveProviderReturnsError(t *testing.T) {
	cfg := FileConfig{
		SelectedProvider: "anthropic",
		Providers:        map[string]ProviderConfig{"anthropic": {}},
	}
	if err := RemoveProvider(&cfg, "anthropic"); err == nil {
		t.Fatal("expected error when removing the selected provider")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	return path
}

func assertEqual(t *testing.T, label, want, got string) {
	t.Helper()
	if want != got {
		t.Errorf("%s: want %q, got %q", label, want, got)
	}
}

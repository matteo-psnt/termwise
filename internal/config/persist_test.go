package config

import (
	"bytes"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// saveOver writes orig to a config file, loads it, applies fn, saves, and
// returns the file's new contents. It fails the test if the save fell back to
// a full rewrite.
func saveOver(t *testing.T, orig string, fn func(*FileConfig)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	fn(&cfg)
	var warn bytes.Buffer
	saveWarnings = &warn
	t.Cleanup(func() { saveWarnings = os.Stderr })
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	if warn.Len() > 0 {
		t.Fatalf("save fell back to a rewrite: %s", warn.String())
	}
	got, _ := os.ReadFile(path)
	reloaded, _, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("saved file does not load: %v\n%s", err, got)
	}
	if a, b := mustMarshal(t, reloaded), mustMarshal(t, cfg); a != b {
		t.Fatalf("saved file decodes to\n%s\nwant\n%s", a, b)
	}
	return string(got)
}

func mustMarshal(t *testing.T, cfg FileConfig) string {
	t.Helper()
	out, err := spliceCanon(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

const userConfig = `# My termwise config
selected_provider = "anthropic"   # the one I use

# Work account
[providers.anthropic]
auth = "env"
env_var = "ANTHROPIC_API_KEY"
model = 'claude-sonnet'  # literal string
future_key = 42

[providers.openai]
# personal
model = "gpt-4o"

[unknown_table]
x = [1, 2, 3]
`

func TestSaveKeepsUnchangedFileByteForByte(t *testing.T) {
	got := saveOver(t, userConfig, func(*FileConfig) {})
	if got != userConfig {
		t.Errorf("unchanged save rewrote the file:\n%s", got)
	}
}

func TestSaveChangesValueInPlace(t *testing.T) {
	got := saveOver(t, userConfig, func(c *FileConfig) { _ = SetModel(c, "anthropic", "claude-opus") })
	want := strings.Replace(userConfig, `'claude-sonnet'`, `"claude-opus"`, 1)
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestSaveChangesSelectedProvider(t *testing.T) {
	got := saveOver(t, userConfig, func(c *FileConfig) { c.SelectedProvider = "openai" })
	want := strings.Replace(userConfig, `selected_provider = "anthropic"`, `selected_provider = "openai"`, 1)
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestSaveRemovesClearedKey(t *testing.T) {
	got := saveOver(t, userConfig, func(c *FileConfig) {
		_ = SetAuthCmd(c, "anthropic", "pass show anthropic")
	})
	want := strings.Replace(userConfig,
		"auth = \"env\"\nenv_var = \"ANTHROPIC_API_KEY\"\n",
		"auth = \"cmd\"\n", 1)
	want = strings.Replace(want, "future_key = 42\n", "future_key = 42\napi_key_cmd = \"pass show anthropic\"\n", 1)
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestSaveAddsProviderTableAtEnd(t *testing.T) {
	got := saveOver(t, userConfig, func(c *FileConfig) { _ = SetModel(c, "ollama", "llama3") })
	want := userConfig + "\n[providers.ollama]\nmodel = \"llama3\"\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestSaveAddsEmptyProviderTable(t *testing.T) {
	got := saveOver(t, userConfig, func(c *FileConfig) { SetProvider(c, "ollama", ProviderConfig{}) })
	want := userConfig + "\n[providers.ollama]\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestSaveRemovesProviderSection(t *testing.T) {
	got := saveOver(t, userConfig, func(c *FileConfig) { _ = RemoveProvider(c, "openai") })
	want := strings.Replace(userConfig, "[providers.openai]\n# personal\nmodel = \"gpt-4o\"\n\n", "", 1)
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestSaveRemovesLastProviderSection(t *testing.T) {
	orig := "selected_provider = \"anthropic\"\n\n[providers.anthropic]\nmodel = \"a\"\n\n[providers.openai]\nmodel = \"b\"\n"
	got := saveOver(t, orig, func(c *FileConfig) { _ = RemoveProvider(c, "openai") })
	want := "selected_provider = \"anthropic\"\n\n[providers.anthropic]\nmodel = \"a\"\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSaveCreatesSettingsTable(t *testing.T) {
	got := saveOver(t, userConfig, func(c *FileConfig) {
		on := true
		c.Settings.AutoResume = &on
		c.Settings.Theme = "dark"
	})
	want := userConfig + "\n[settings]\ntheme = \"dark\"\nauto_resume = true\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestSaveAddsKeyToExistingTableWithItsIndent(t *testing.T) {
	orig := "[settings]\n  theme = \"dark\"\n\n[providers.openai]\nmodel = \"x\"\n"
	got := saveOver(t, orig, func(c *FileConfig) {
		off := false
		c.Settings.LLMJudge = &off
	})
	want := "[settings]\n  theme = \"dark\"\n  llm_judge = false\n\n[providers.openai]\nmodel = \"x\"\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSaveAddsSelectedProviderAboveTables(t *testing.T) {
	orig := "# termwise\n\n# providers below\n[providers.openai]\nmodel = \"x\"\n"
	got := saveOver(t, orig, func(c *FileConfig) { c.SelectedProvider = "openai" })
	want := "# termwise\n\nselected_provider = \"openai\"\n\n# providers below\n[providers.openai]\nmodel = \"x\"\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSaveRoundTripsAwkwardStrings(t *testing.T) {
	for _, v := range []string{
		`say "hi"`, `it's`, `both ' and "`, `C:\path\to`, "naïve ✓ 日本", "tab\there", "line\nbreak", `\"`, "'''",
	} {
		got := saveOver(t, userConfig, func(c *FileConfig) { _ = SetAuthCmd(c, "openai", v) })
		cfg := FileConfig{}
		if err := decodeForTest(got, &cfg); err != nil {
			t.Fatalf("%q: %v", v, err)
		}
		if cfg.Providers["openai"].APIKeyCmd != v {
			t.Errorf("api_key_cmd = %q, want %q", cfg.Providers["openai"].APIKeyCmd, v)
		}
		if !strings.Contains(got, "# personal\n") || !strings.Contains(got, "future_key = 42") {
			t.Errorf("%q: lost user content:\n%s", v, got)
		}
		// Saving the loaded result again changes nothing.
		if again := saveOver(t, got, func(*FileConfig) {}); again != got {
			t.Errorf("%q: second save changed the file:\n%s", v, again)
		}
	}
}

func TestSaveKeepsCRLF(t *testing.T) {
	orig := "[providers.openai]\r\nmodel = \"x\"\r\n"
	got := saveOver(t, orig, func(c *FileConfig) {
		_ = SetAuthEnv(c, "openai", "MY_KEY")
		c.SelectedProvider = "openai"
	})
	want := "selected_provider = \"openai\"\r\n\r\n[providers.openai]\r\nmodel = \"x\"\r\nauth = \"env\"\r\nenv_var = \"MY_KEY\"\r\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSaveFileWithoutTrailingNewline(t *testing.T) {
	orig := "[providers.openai]\nmodel = \"x\""
	got := saveOver(t, orig, func(c *FileConfig) { _ = SetBaseURL(c, "openai", "http://localhost:1") })
	want := "[providers.openai]\nmodel = \"x\"\nbase_url = \"http://localhost:1\"\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSaveFallsBackForInlineTableAndKeepsBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	orig := "# mine\nproviders = { openai = { model = \"x\" } }\n"
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ := LoadConfig(path)
	_ = SetModel(&cfg, "openai", "y")
	var warn bytes.Buffer
	saveWarnings = &warn
	t.Cleanup(func() { saveWarnings = os.Stderr })
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warn.String(), "config.toml.bak") || strings.Count(warn.String(), "\n") != 1 {
		t.Errorf("warning = %q, want one line naming the backup", warn.String())
	}
	if bak, _ := os.ReadFile(path + ".bak"); string(bak) != orig {
		t.Errorf("backup = %q, want %q", bak, orig)
	}
	got, _, err := LoadConfig(path)
	if err != nil || got.Providers["openai"].Model != "y" {
		t.Errorf("rewritten config = %+v, %v", got, err)
	}
}

func TestSaveFallsBackForDottedKey(t *testing.T) {
	orig := "providers.openai.model = \"x\"\n"
	path := filepath.Join(t.TempDir(), "config.toml")
	_ = os.WriteFile(path, []byte(orig), 0o600)
	cfg, _, _ := LoadConfig(path)
	cfg.Providers["openai"] = ProviderConfig{}
	var warn bytes.Buffer
	saveWarnings = &warn
	t.Cleanup(func() { saveWarnings = os.Stderr })
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	if warn.Len() == 0 {
		t.Error("no fallback warning for a dotted key that had to be removed")
	}
	got, _, _ := LoadConfig(path)
	if pc, ok := got.Providers["openai"]; !ok || pc.Model != "" {
		t.Errorf("providers = %+v", got.Providers)
	}
}

func spliceCanon(cfg FileConfig) (string, error) {
	out, err := toml.Marshal(cfg)
	return string(out), err
}

func decodeForTest(s string, cfg *FileConfig) error {
	return toml.Unmarshal([]byte(s), cfg)
}

func TestSaveClearingSettingKeepsTableAndOtherKeys(t *testing.T) {
	orig := "[settings]\n# keep me\nauto_resume = true\nmystery = \"x\"\n"
	got := saveOver(t, orig, func(c *FileConfig) { c.Settings.AutoResume = nil })
	want := "[settings]\n# keep me\nmystery = \"x\"\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSaveFlipsBoolInPlace(t *testing.T) {
	orig := "[settings]\nauto_resume = true # yes\n"
	got := saveOver(t, orig, func(c *FileConfig) {
		off := false
		c.Settings.AutoResume = &off
	})
	if want := "[settings]\nauto_resume = false # yes\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSaveRenamesOnlyProviderAndSelectsIt(t *testing.T) {
	// Dropping [providers.a] walks back over the blank line above it, to
	// where selected_provider is inserted: the two edits used to overlap
	// and the save fell back to a rewrite that lost the comment.
	orig := "# termwise\n\n[providers.a]\nmodel = \"m\"\n"
	got := saveOver(t, orig, func(c *FileConfig) {
		pc := c.Providers["a"]
		delete(c.Providers, "a")
		c.Providers["b"] = pc
		c.SelectedProvider = "b"
	})
	want := "# termwise\n\nselected_provider = \"b\"\n\n[providers.b]\nmodel = \"m\"\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestSpliceProperty saves random changes over plain hand-written files —
// comments, blank lines, CRLF, no final newline, but no dotted keys, inline
// tables or arrays of tables — and checks each one is edited in place and
// decodes to the config that was saved.
func TestSpliceProperty(t *testing.T) {
	bases := []string{
		"",
		"# termwise\n",
		"# termwise\n\n[providers.a]\nmodel = \"m\"\n",
		"# termwise\n\n\n[providers.a]\nmodel = \"m\"\n\n",
		"selected_provider = \"a\"\n\n[providers.a]\nmodel = \"m\"\n\n# openai\n[providers.openai]\nauth = \"env\"\nmodel = 'gpt'  # literal\n\n[settings]\ntheme = \"dark\"\n",
		"[settings]\n  auto_resume = true\n\n[providers.ollama]\n",
		"# top\n\n# about a\n[providers.a]\nmodel = \"m\"\nmystery = 1\n\n[providers.b]\nmodel = \"n\"",
		userConfig,
	}
	for _, b := range bases[:len(bases):len(bases)] {
		bases = append(bases, strings.ReplaceAll(b, "\n", "\r\n"))
	}
	names := []string{"a", "b", "openai", "anthropic", "ollama"}
	values := []string{"", "m", "gpt-4o", `say "hi"`, `C:\path`, "it's", "naïve"}
	rng := rand.New(rand.NewPCG(1, 2))
	pick := func(xs []string) string { return xs[rng.IntN(len(xs))] }
	boolp := func() *bool {
		if rng.IntN(3) == 0 {
			return nil
		}
		v := rng.IntN(2) == 0
		return &v
	}
	mutate := func(c *FileConfig) {
		if c.Providers == nil {
			c.Providers = map[string]ProviderConfig{}
		}
		switch rng.IntN(9) {
		case 0:
			c.SelectedProvider = pick(append([]string{""}, names...))
		case 1:
			c.Providers[pick(names)] = ProviderConfig{Model: pick(values)}
		case 2:
			delete(c.Providers, pick(names))
		case 3: // rename
			from, to := pick(names), pick(names)
			if pc, ok := c.Providers[from]; ok {
				delete(c.Providers, from)
				c.Providers[to] = pc
				c.SelectedProvider = to
			}
		case 4:
			for n, pc := range c.Providers {
				pc.Model, pc.BaseURL, pc.Effort = pick(values), pick(values), pick(append([]string{""}, EffortLevels...))
				c.Providers[n] = pc
				break
			}
		case 5:
			for n, pc := range c.Providers {
				pc.Auth, pc.APIKeyCmd, pc.EnvVar = pick([]string{"", "env", "cmd", "keychain"}), pick(values), pick(values)
				c.Providers[n] = pc
				break
			}
		case 6:
			c.Settings.Theme, c.Settings.Keybinding = pick(values), pick(values)
		case 7:
			c.Settings.AutoResume, c.Settings.LLMJudge = boolp(), boolp()
		case 8:
			c.Settings.Suggestions, c.Settings.ShellContext = boolp(), boolp()
		}
	}
	for i := range 3000 {
		base := bases[rng.IntN(len(bases))]
		var cfg FileConfig
		if err := toml.Unmarshal([]byte(base), &cfg); err != nil {
			t.Fatal(err)
		}
		for range 1 + rng.IntN(4) {
			mutate(&cfg)
		}
		if _, err := spliceConfig([]byte(base), cfg); err != nil {
			want, _ := toml.Marshal(cfg)
			t.Fatalf("case %d: splice failed: %v\nfile:\n%q\nsaving:\n%s", i, err, base, want)
		}
	}
}

func TestSaveRemovesTwoAdjacentProviderSections(t *testing.T) {
	// Both sections claim the blank line between them.
	orig := "# top\n\n[providers.a]\nmodel = \"m\"\n\n[providers.b]\nmodel = \"n\"\n"
	got := saveOver(t, orig, func(c *FileConfig) {
		_ = RemoveProvider(c, "a")
		_ = RemoveProvider(c, "b")
		c.SelectedProvider = "openai"
	})
	if want := "# top\n\nselected_provider = \"openai\"\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

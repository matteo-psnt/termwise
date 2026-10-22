package agentui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/theme"
)

// /theme was typed five times in real use and every one of those was sent to
// the model as an ordinary prompt, because only /effort, /model, /config,
// /clear and /help were registered.
func TestThemeIsARegisteredSlashCommand(t *testing.T) {
	cmd := findSlashCommand("/theme")
	if cmd == nil {
		t.Fatal("/theme is not registered")
	}
	if cmd.Run == nil || cmd.Preview == nil || cmd.ArgOptions == nil {
		t.Error("/theme should have a runner, a live preview, and arg completion like /model does")
	}
	if len(cmd.ArgOptions(Model{})) == 0 {
		t.Error("/theme offers no theme names for completion")
	}
}

// Every setting that has its own slash command should complete to real values.
func TestThemeArgOptionsAreValidThemes(t *testing.T) {
	for _, name := range themeArgOptions(Model{}) {
		if !theme.IsValid(name) {
			t.Errorf("%q is offered for completion but is not a valid theme", name)
		}
	}
}

func TestUnknownSlashCommandStillFallsThrough(t *testing.T) {
	if findSlashCommand("/definitely-not-a-command") != nil {
		t.Error("unknown command unexpectedly resolved")
	}
}

// /model completion reads the config file; repeated lookups for the same input
// must come from the cache rather than re-reading it.
func TestModelArgCompletionIsCachedPerInputValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := config.FileConfig{}
	config.SetProvider(&cfg, "anthropic", config.ProviderConfig{})
	if err := config.SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	m := pickerModel(theme.DefaultName)
	m.cfgPath = path
	m.providerName = "anthropic"
	m.slashCache = &slashMatchCache{}

	m.input.SetValue("/model anthropic/")
	first := m.visibleSlashMatches()
	if len(first) == 0 {
		t.Fatal("expected /model completions from the config")
	}

	// With the file gone, only a cached result can still list models.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if again := m.visibleSlashMatches(); len(again) != len(first) {
		t.Fatalf("same input re-read the config: got %d matches, want %d", len(again), len(first))
	}

	m.input.SetValue("/model anthropic/c")
	if changed := m.visibleSlashMatches(); len(changed) != 0 {
		t.Fatalf("a new input value served stale matches: %v", changed)
	}
}
